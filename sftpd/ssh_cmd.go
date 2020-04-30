package sftpd

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/drakkan/sftpgo/dataprovider"
	"github.com/drakkan/sftpgo/logger"
	"github.com/drakkan/sftpgo/metrics"
	"github.com/drakkan/sftpgo/utils"
	"github.com/drakkan/sftpgo/vfs"
	"github.com/google/shlex"
	"golang.org/x/crypto/ssh"
)

const scpCmdName = "scp"

var (
	errQuotaExceeded     = errors.New("denying write due to space limit")
	errPermissionDenied  = errors.New("Permission denied. You don't have the permissions to execute this command")
	errUnsupportedConfig = errors.New("command unsupported for this configuration")
)

type sshCommand struct {
	command    string
	args       []string
	connection Connection
}

type systemCommand struct {
	cmd      *exec.Cmd
	realPath string
}

func processSSHCommand(payload []byte, connection *Connection, channel ssh.Channel, enabledSSHCommands []string) bool {
	var msg sshSubsystemExecMsg
	if err := ssh.Unmarshal(payload, &msg); err == nil {
		name, args, err := parseCommandPayload(msg.Command)
		connection.Log(logger.LevelDebug, logSenderSSH, "new ssh command: %#v args: %v num args: %v user: %v, error: %v",
			name, args, len(args), connection.User.Username, err)
		if err == nil && utils.IsStringInSlice(name, enabledSSHCommands) {
			connection.command = msg.Command
			if name == scpCmdName && len(args) >= 2 {
				connection.protocol = protocolSCP
				connection.channel = channel
				scpCommand := scpCommand{
					sshCommand: sshCommand{
						command:    name,
						connection: *connection,
						args:       args},
				}
				go scpCommand.handle() //nolint:errcheck
				return true
			}
			if name != scpCmdName {
				connection.protocol = protocolSSH
				connection.channel = channel
				sshCommand := sshCommand{
					command:    name,
					connection: *connection,
					args:       args,
				}
				go sshCommand.handle() //nolint:errcheck
				return true
			}
		} else {
			connection.Log(logger.LevelInfo, logSenderSSH, "ssh command not enabled/supported: %#v", name)
		}
	}
	return false
}

func (c *sshCommand) handle() error {
	addConnection(c.connection)
	defer removeConnection(c.connection)
	updateConnectionActivity(c.connection.ID)
	if utils.IsStringInSlice(c.command, sshHashCommands) {
		return c.handleHashCommands()
	} else if utils.IsStringInSlice(c.command, systemCommands) {
		command, err := c.getSystemCommand()
		if err != nil {
			return c.sendErrorResponse(err)
		}
		return c.executeSystemCommand(command)
	} else if c.command == "cd" {
		c.sendExitStatus(nil)
	} else if c.command == "pwd" {
		// hard coded response to "/"
		c.connection.channel.Write([]byte("/\n")) //nolint:errcheck
		c.sendExitStatus(nil)
	}
	return nil
}

func (c *sshCommand) handleHashCommands() error {
	if !vfs.IsLocalOsFs(c.connection.fs) {
		return c.sendErrorResponse(errUnsupportedConfig)
	}
	var h hash.Hash
	if c.command == "md5sum" {
		h = md5.New()
	} else if c.command == "sha1sum" {
		h = sha1.New()
	} else if c.command == "sha256sum" {
		h = sha256.New()
	} else if c.command == "sha384sum" {
		h = sha512.New384()
	} else {
		h = sha512.New()
	}
	var response string
	if len(c.args) == 0 {
		// without args we need to read the string to hash from stdin
		buf := make([]byte, 4096)
		n, err := c.connection.channel.Read(buf)
		if err != nil && err != io.EOF {
			return c.sendErrorResponse(err)
		}
		h.Write(buf[:n]) //nolint:errcheck
		response = fmt.Sprintf("%x  -\n", h.Sum(nil))
	} else {
		sshPath := c.getDestPath()
		if !c.connection.User.IsFileAllowed(sshPath) {
			c.connection.Log(logger.LevelInfo, logSenderSSH, "hash not allowed for file %#v", sshPath)
			return c.sendErrorResponse(errPermissionDenied)
		}
		fsPath, err := c.connection.fs.ResolvePath(sshPath)
		if err != nil {
			return c.sendErrorResponse(err)
		}
		if !c.connection.User.HasPerm(dataprovider.PermListItems, sshPath) {
			return c.sendErrorResponse(errPermissionDenied)
		}
		hash, err := computeHashForFile(h, fsPath)
		if err != nil {
			return c.sendErrorResponse(err)
		}
		response = fmt.Sprintf("%v  %v\n", hash, sshPath)
	}
	c.connection.channel.Write([]byte(response)) //nolint:errcheck
	c.sendExitStatus(nil)
	return nil
}

func (c *sshCommand) executeSystemCommand(command systemCommand) error {
	if !vfs.IsLocalOsFs(c.connection.fs) {
		return c.sendErrorResponse(errUnsupportedConfig)
	}
	if c.connection.User.QuotaFiles > 0 && c.connection.User.UsedQuotaFiles > c.connection.User.QuotaFiles {
		return c.sendErrorResponse(errQuotaExceeded)
	}
	perms := []string{dataprovider.PermDownload, dataprovider.PermUpload, dataprovider.PermCreateDirs, dataprovider.PermListItems,
		dataprovider.PermOverwrite, dataprovider.PermDelete, dataprovider.PermRename}
	if !c.connection.User.HasPerms(perms, c.getDestPath()) {
		return c.sendErrorResponse(errPermissionDenied)
	}

	stdin, err := command.cmd.StdinPipe()
	if err != nil {
		return c.sendErrorResponse(err)
	}
	stdout, err := command.cmd.StdoutPipe()
	if err != nil {
		return c.sendErrorResponse(err)
	}
	stderr, err := command.cmd.StderrPipe()
	if err != nil {
		return c.sendErrorResponse(err)
	}
	err = command.cmd.Start()
	if err != nil {
		return c.sendErrorResponse(err)
	}

	closeCmdOnError := func() {
		c.connection.Log(logger.LevelDebug, logSenderSSH, "kill cmd: %#v and close ssh channel after read or write error",
			c.connection.command)
		killerr := command.cmd.Process.Kill()
		closerr := c.connection.channel.Close()
		c.connection.Log(logger.LevelDebug, logSenderSSH, "kill cmd error: %v close channel error: %v", killerr, closerr)
	}
	var once sync.Once
	commandResponse := make(chan bool)

	go func() {
		defer stdin.Close()
		remainingQuotaSize := int64(0)
		if c.connection.User.QuotaSize > 0 {
			remainingQuotaSize = c.connection.User.QuotaSize - c.connection.User.UsedQuotaSize
		}
		transfer := Transfer{
			file:           nil,
			path:           command.realPath,
			start:          time.Now(),
			bytesSent:      0,
			bytesReceived:  0,
			user:           c.connection.User,
			connectionID:   c.connection.ID,
			transferType:   transferUpload,
			lastActivity:   time.Now(),
			isNewFile:      false,
			protocol:       c.connection.protocol,
			transferError:  nil,
			isFinished:     false,
			minWriteOffset: 0,
			lock:           new(sync.Mutex),
		}
		addTransfer(&transfer)
		defer removeTransfer(&transfer) //nolint:errcheck
		w, e := transfer.copyFromReaderToWriter(stdin, c.connection.channel, remainingQuotaSize)
		c.connection.Log(logger.LevelDebug, logSenderSSH, "command: %#v, copy from remote command to sdtin ended, written: %v, "+
			"initial remaining quota: %v, err: %v", c.connection.command, w, remainingQuotaSize, e)
		if e != nil {
			once.Do(closeCmdOnError)
		}
	}()

	go func() {
		transfer := Transfer{
			file:           nil,
			path:           command.realPath,
			start:          time.Now(),
			bytesSent:      0,
			bytesReceived:  0,
			user:           c.connection.User,
			connectionID:   c.connection.ID,
			transferType:   transferDownload,
			lastActivity:   time.Now(),
			isNewFile:      false,
			protocol:       c.connection.protocol,
			transferError:  nil,
			isFinished:     false,
			minWriteOffset: 0,
			lock:           new(sync.Mutex),
		}
		addTransfer(&transfer)
		defer removeTransfer(&transfer) //nolint:errcheck
		w, e := transfer.copyFromReaderToWriter(c.connection.channel, stdout, 0)
		c.connection.Log(logger.LevelDebug, logSenderSSH, "command: %#v, copy from sdtout to remote command ended, written: %v err: %v",
			c.connection.command, w, e)
		if e != nil {
			once.Do(closeCmdOnError)
		}
		commandResponse <- true
	}()

	go func() {
		transfer := Transfer{
			file:           nil,
			path:           command.realPath,
			start:          time.Now(),
			bytesSent:      0,
			bytesReceived:  0,
			user:           c.connection.User,
			connectionID:   c.connection.ID,
			transferType:   transferDownload,
			lastActivity:   time.Now(),
			isNewFile:      false,
			protocol:       c.connection.protocol,
			transferError:  nil,
			isFinished:     false,
			minWriteOffset: 0,
			lock:           new(sync.Mutex),
		}
		addTransfer(&transfer)
		defer removeTransfer(&transfer) //nolint:errcheck
		w, e := transfer.copyFromReaderToWriter(c.connection.channel.Stderr(), stderr, 0)
		c.connection.Log(logger.LevelDebug, logSenderSSH, "command: %#v, copy from sdterr to remote command ended, written: %v err: %v",
			c.connection.command, w, e)
		// os.ErrClosed means that the command is finished so we don't need to do anything
		if (e != nil && !errors.Is(e, os.ErrClosed)) || w > 0 {
			once.Do(closeCmdOnError)
		}
	}()

	<-commandResponse
	err = command.cmd.Wait()
	c.sendExitStatus(err)
	c.rescanHomeDir() //nolint:errcheck
	return err
}

func (c *sshCommand) checkGitAllowed() error {
	gitPath := c.getDestPath()
	for _, v := range c.connection.User.VirtualFolders {
		if v.VirtualPath == gitPath {
			c.connection.Log(logger.LevelDebug, logSenderSSH, "git is not supported inside virtual folder %#v user %#v",
				gitPath, c.connection.User.Username)
			return errUnsupportedConfig
		}
		if len(gitPath) > len(v.VirtualPath) {
			if strings.HasPrefix(gitPath, v.VirtualPath+"/") {
				c.connection.Log(logger.LevelDebug, logSenderSSH, "git is not supported inside virtual folder %#v user %#v",
					gitPath, c.connection.User.Username)
				return errUnsupportedConfig
			}
		}
	}
	for _, f := range c.connection.User.Filters.FileExtensions {
		if f.Path == gitPath {
			c.connection.Log(logger.LevelDebug, logSenderSSH,
				"git is not supported inside folder with files extensions filters %#v user %#v", gitPath,
				c.connection.User.Username)
			return errUnsupportedConfig
		}
		if len(gitPath) > len(f.Path) {
			if strings.HasPrefix(gitPath, f.Path+"/") || f.Path == "/" {
				c.connection.Log(logger.LevelDebug, logSenderSSH,
					"git is not supported inside folder with files extensions filters %#v user %#v", gitPath,
					c.connection.User.Username)
				return errUnsupportedConfig
			}
		}
	}
	return nil
}

func (c *sshCommand) getSystemCommand() (systemCommand, error) {
	command := systemCommand{
		cmd:      nil,
		realPath: "",
	}
	args := make([]string, len(c.args))
	copy(args, c.args)
	var path string
	if len(c.args) > 0 {
		var err error
		sshPath := c.getDestPath()
		path, err = c.connection.fs.ResolvePath(sshPath)
		if err != nil {
			return command, err
		}
		args = args[:len(args)-1]
		args = append(args, path)
	}
	if strings.HasPrefix(c.command, "git-") {
		// we don't allow git inside virtual folders or folders with files extensions filters
		if err := c.checkGitAllowed(); err != nil {
			return command, err
		}
	}
	if c.command == "rsync" {
		// if the user has virtual folders or file extensions filters we don't allow rsync since the rsync command
		// interacts with the filesystem directly and it is not aware about virtual folders/extensions files filters
		if len(c.connection.User.VirtualFolders) > 0 {
			c.connection.Log(logger.LevelDebug, logSenderSSH, "user %#v has virtual folders, rsync is not supported",
				c.connection.User.Username)
			return command, errUnsupportedConfig
		}
		if len(c.connection.User.Filters.FileExtensions) > 0 {
			c.connection.Log(logger.LevelDebug, logSenderSSH, "user %#v has file extensions filter, rsync is not supported",
				c.connection.User.Username)
			return command, errUnsupportedConfig
		}
		// we cannot avoid that rsync creates symlinks so if the user has the permission
		// to create symlinks we add the option --safe-links to the received rsync command if
		// it is not already set. This should prevent to create symlinks that point outside
		// the home dir.
		// If the user cannot create symlinks we add the option --munge-links, if it is not
		// already set. This should make symlinks unusable (but manually recoverable)
		if c.connection.User.HasPerm(dataprovider.PermCreateSymlinks, c.getDestPath()) {
			if !utils.IsStringInSlice("--safe-links", args) {
				args = append([]string{"--safe-links"}, args...)
			}
		} else {
			if !utils.IsStringInSlice("--munge-links", args) {
				args = append([]string{"--munge-links"}, args...)
			}
		}
	}
	c.connection.Log(logger.LevelDebug, logSenderSSH, "new system command %#v, with args: %v path: %v", c.command, args, path)
	cmd := exec.Command(c.command, args...)
	uid := c.connection.User.GetUID()
	gid := c.connection.User.GetGID()
	cmd = wrapCmd(cmd, uid, gid)
	command.cmd = cmd
	command.realPath = path
	return command, nil
}

func (c *sshCommand) rescanHomeDir() error {
	quotaTracking := dataprovider.GetQuotaTracking()
	if (!c.connection.User.HasQuotaRestrictions() && quotaTracking == 2) || quotaTracking == 0 {
		return nil
	}
	var err error
	var numFiles int
	var size int64
	if AddQuotaScan(c.connection.User.Username) {
		numFiles, size, err = c.connection.fs.ScanRootDirContents()
		if err != nil {
			c.connection.Log(logger.LevelWarn, logSenderSSH, "error scanning user home dir %#v: %v", c.connection.User.HomeDir, err)
		} else {
			err := dataprovider.UpdateUserQuota(dataProvider, c.connection.User, numFiles, size, true)
			c.connection.Log(logger.LevelDebug, logSenderSSH, "user home dir scanned, user: %#v, dir: %#v, error: %v",
				c.connection.User.Username, c.connection.User.HomeDir, err)
		}
		RemoveQuotaScan(c.connection.User.Username) //nolint:errcheck
	}
	return err
}

// for the supported command, the path, if any, is the last argument
func (c *sshCommand) getDestPath() string {
	if len(c.args) == 0 {
		return ""
	}
	destPath := strings.Trim(c.args[len(c.args)-1], "'")
	destPath = strings.Trim(destPath, "\"")
	result := utils.CleanSFTPPath(destPath)
	if strings.HasSuffix(destPath, "/") && !strings.HasSuffix(result, "/") {
		result += "/"
	}
	return result
}

// we try to avoid to leak the real filesystem path here
func (c *sshCommand) getMappedError(err error) error {
	if c.connection.fs.IsNotExist(err) {
		return errors.New("no such file or directory")
	}
	if c.connection.fs.IsPermission(err) {
		return errors.New("permission denied")
	}
	return err
}

func (c *sshCommand) sendErrorResponse(err error) error {
	errorString := fmt.Sprintf("%v: %v %v\n", c.command, c.getDestPath(), c.getMappedError(err))
	c.connection.channel.Write([]byte(errorString)) //nolint:errcheck
	c.sendExitStatus(err)
	return err
}

func (c *sshCommand) sendExitStatus(err error) {
	status := uint32(0)
	if err != nil {
		status = uint32(1)
		c.connection.Log(logger.LevelWarn, logSenderSSH, "command failed: %#v args: %v user: %v err: %v",
			c.command, c.args, c.connection.User.Username, err)
	} else {
		logger.CommandLog(sshCommandLogSender, c.getDestPath(), "", c.connection.User.Username, "", c.connection.ID,
			protocolSSH, -1, -1, "", "", c.connection.command)
	}
	exitStatus := sshSubsystemExitStatus{
		Status: status,
	}
	c.connection.channel.SendRequest("exit-status", false, ssh.Marshal(&exitStatus)) //nolint:errcheck
	c.connection.channel.Close()
	// for scp we notify single uploads/downloads
	if c.command != scpCmdName {
		metrics.SSHCommandCompleted(err)
		realPath := c.getDestPath()
		if len(realPath) > 0 {
			p, e := c.connection.fs.ResolvePath(realPath)
			if e == nil {
				realPath = p
			}
		}
		go executeAction(newActionNotification(c.connection.User, operationSSHCmd, realPath, "", c.command, 0, err)) //nolint:errcheck
	}
}

func computeHashForFile(hasher hash.Hash, path string) (string, error) {
	hash := ""
	f, err := os.Open(path)
	if err != nil {
		return hash, err
	}
	defer f.Close()
	_, err = io.Copy(hasher, f)
	if err == nil {
		hash = fmt.Sprintf("%x", hasher.Sum(nil))
	}
	return hash, err
}

func parseCommandPayload(command string) (string, []string, error) {
	parts, err := shlex.Split(command)
	if err == nil && len(parts) == 0 {
		err = fmt.Errorf("invalid command: %#v", command)
	}
	if err != nil {
		return "", []string{}, err
	}
	if len(parts) < 2 {
		return parts[0], []string{}, nil
	}
	return parts[0], parts[1:], nil
}
