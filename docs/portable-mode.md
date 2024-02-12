# Portable mode

SFTPGo allows to share a single directory (and multiple virtual directory mappings) on demand using the `portable` subcommand:

```console
sftpgo portable --help
To serve the current working directory with auto generated credentials simply
use:

$ sftpgo portable

Please take a look at the usage below to customize the serving parameters

Usage:
  sftpgo portable [flags]

Flags:
      --allowed-patterns stringArray    Allowed file patterns case insensitive.
                                        The format is:
                                        /dir::pattern1,pattern2.
                                        For example: "/somedir::*.jpg,a*b?.png"
      --az-access-tier string           Leave empty to use the default
                                        container setting
      --az-account-key string
      --az-account-name string
      --az-container string
      --az-download-concurrency int     How many parts are downloaded in
                                        parallel (default 5)
      --az-download-part-size int       The buffer size for multipart downloads
                                        (MB) (default 5)
      --az-endpoint string              Leave empty to use the default:
                                        "blob.core.windows.net"
      --az-key-prefix string            Allows to restrict access to the
                                        virtual folder identified by this
                                        prefix and its contents
      --az-sas-url string               Shared access signature URL
      --az-upload-concurrency int       How many parts are uploaded in
                                        parallel (default 5)
      --az-upload-part-size int         The buffer size for multipart uploads
                                        (MB) (default 5)
      --az-use-emulator
  -c, --config-dir string               Location of the config dir. This directory
                                        is used as the base for files with a relative
                                        path, e.g. the private keys for the SFTP
                                        server or the database file if you use a
                                        file-based data provider.
                                        The configuration file, if not explicitly set,
                                        is looked for in this dir. We support reading
                                        from JSON, TOML, YAML, HCL, envfile and Java
                                        properties config files. The default config
                                        file name is "sftpgo" and therefore
                                        "sftpgo.json", "sftpgo.yaml" and so on are
                                        searched.
                                        This flag can be set using SFTPGO_CONFIG_DIR
                                        env var too. (default ".")
      --config-file string              Path to SFTPGo configuration file.
                                        This flag explicitly defines the path, name
                                        and extension of the config file. If must be
                                        an absolute path or a path relative to the
                                        configuration directory. The specified file
                                        name must have a supported extension (JSON,
                                        YAML, TOML, HCL or Java properties).
                                        This flag can be set using SFTPGO_CONFIG_FILE
                                        env var too.
      --crypto-passphrase string        Passphrase for encryption/decryption
      --denied-patterns stringArray     Denied file patterns case insensitive.
                                        The format is:
                                        /dir::pattern1,pattern2.
                                        For example: "/somedir::*.jpg,a*b?.png"
  -d, --directory string                Path to the directory to serve.
                                        This can be an absolute path or a path
                                        relative to the current directory
                                         (default ".")
  -f, --fs-provider string              osfs => local filesystem (legacy value: 0)
                                        s3fs => AWS S3 compatible (legacy: 1)
                                        gcsfs => Google Cloud Storage (legacy: 2)
                                        azblobfs => Azure Blob Storage (legacy: 3)
                                        cryptfs => Encrypted local filesystem (legacy: 4)
                                        sftpfs => SFTP (legacy: 5) (default "osfs")
      --ftpd-cert string                Path to the certificate file for FTPS
      --ftpd-key string                 Path to the key file for FTPS
      --ftpd-port int                   0 means a random unprivileged port,
                                        < 0 disabled (default -1)
      --gcs-automatic-credentials int   0 means explicit credentials using
                                        a JSON credentials file, 1 automatic
                                         (default 1)
      --gcs-bucket string
      --gcs-credentials-file string     Google Cloud Storage JSON credentials
                                        file
      --gcs-key-prefix string           Allows to restrict access to the
                                        virtual folder identified by this
                                        prefix and its contents
      --gcs-storage-class string
      --grace-time int                  This grace time defines the number of
                                        seconds allowed for existing transfers
                                        to get completed before shutting down.
                                        A graceful shutdown is triggered by an
                                        interrupt signal.

  -h, --help                            help for portable
      --httpd-cert string               Path to the certificate file for WebClient
                                        over HTTPS
      --httpd-key string                Path to the key file for WebClient over
                                        HTTPS
      --httpd-port int                  0 means a random unprivileged port,
                                        < 0 disabled (default -1)
  -l, --log-file-path string            Leave empty to disable logging
      --log-level string                Set the log level.
                                        Supported values:

                                        debug, info, warn, error.
                                         (default "debug")
      --log-utc-time                    Use UTC time for logging
  -p, --password string                 Leave empty to use an auto generated
                                        value
      --password-file string            Read the password from the specified
                                        file path. Leave empty to use an auto
                                        generated value
  -g, --permissions strings             User's permissions. "*" means any
                                        permission (default [list,download])
  -k, --public-key strings
      --s3-access-key string
      --s3-access-secret string
      --s3-acl string
      --s3-bucket string
      --s3-endpoint string
      --s3-force-path-style             Force path style bucket URL
      --s3-key-prefix string            Allows to restrict access to the
                                        virtual folder identified by this
                                        prefix and its contents
      --s3-region string
      --s3-role-arn string
      --s3-skip-tls-verify              If enabled the S3 client accepts any TLS
                                        certificate presented by the server and
                                        any host name in that certificate.
                                        In this mode, TLS is susceptible to
                                        man-in-the-middle attacks.
                                        This should be used only for testing.

      --s3-storage-class string
      --s3-upload-concurrency int       How many parts are uploaded in
                                        parallel (default 2)
      --s3-upload-part-size int         The buffer size for multipart uploads
                                        (MB) (default 5)
      --sftp-buffer-size int            The size of the buffer (in MB) to use
                                        for transfers. By enabling buffering,
                                        the reads and writes, from/to the
                                        remote SFTP server, are split in
                                        multiple concurrent requests and this
                                        allows data to be transferred at a
                                        faster rate, over high latency networks,
                                        by overlapping round-trip times
      --sftp-disable-concurrent-reads   Concurrent reads are safe to use and
                                        disabling them will degrade performance.
                                        Disable for read once servers
      --sftp-endpoint string            SFTP endpoint as host:port for SFTP
                                        provider
      --sftp-fingerprints strings       SFTP fingerprints to verify remote host
                                        key for SFTP provider
      --sftp-key-path string            SFTP private key path for SFTP provider
      --sftp-password string            SFTP password for SFTP provider
      --sftp-prefix string              SFTP prefix allows restrict all
                                        operations to a given path within the
                                        remote SFTP server
      --sftp-username string            SFTP user for SFTP provider
  -s, --sftpd-port int                  0 means a random unprivileged port,
                                        < 0 disabled
      --ssh-commands strings            SSH commands to enable.
                                        "*" means any supported SSH command
                                        including scp
                                         (default [md5sum,sha1sum,sha256sum,cd,pwd,scp])
      --start-directory string          Alternate start directory.
                                        This is a virtual path not a filesystem
                                        path (default "/")
  -u, --username string                 Leave empty to use an auto generated
                                        value
  -v, --virtual-directory strings       virtual directory mapping: "-v <host-path>[:<virtual-path>[:<permission>[;<permission>]]]".
                                        available permissions: "*,list,download,upload,overwrite,delete,rename,create_dirs,create_symlinks,chmod,chown,chtimes"
      --webdav-cert string              Path to the certificate file for WebDAV
                                        over HTTPS
      --webdav-key string               Path to the key file for WebDAV over
                                        HTTPS
      --webdav-port int                 0 means a random unprivileged port,
                                        < 0 disabled (default -1)
```

In portable mode you can apply further customizations using a configuration file/environment variables as for the service mode.
SFTP, FTP, HTTP and WebDAV settings configured using the CLI flags are applied to the first binding, any additional bindings will not be affected.
