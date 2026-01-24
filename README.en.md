# ClearVault

English | [简体中文](README.md)

ClearVault is an encrypted cloud storage proxy service based on the WebDAV protocol. It encrypts files before storing them on any WebDAV-compatible cloud storage service (such as Nextcloud, Nutstore, etc.) while providing a local WebDAV interface for client access.

## ✨ Key Features

- 🔐 **End-to-End Encryption**: AES-256-GCM encryption algorithm with user-controlled master key
- 🌐 **WebDAV Protocol**: Compatible with all WebDAV clients (RaiDrive, Windows Explorer, macOS Finder, etc.)
- 📁 **Filename Encryption**: Complete encryption of filenames and directory structure; remote storage only saves random hashes
- 🚀 **Streaming Encryption/Decryption**: Supports streaming processing for large files with low memory usage
- 💾 **Flexible Metadata Storage**: Uses local filesystem (JSON) for metadata storage, simple and reliable
- 🔄 **Full WebDAV Support**: Supports file upload, download, delete, rename, directory operations, etc.
- 🪟 **Windows Optimization**: Special optimizations for Windows file locking and RaiDrive client
- 📤 **Offline Encrypted Export**: Encrypt files locally and manually upload to cloud when WebDAV uploads are unstable
- 🌍 **S3 Protocol Support**: Supports S3-compatible storage (MinIO, Cloudflare R2, AWS S3, etc.) as remote storage
- 📤📤 **Simple Share Feature**: Export/import encrypted metadata packages for secure file sharing
- 🧩 **FUSE Mount**: Mount encrypted storage as a local directory (for NAS/system integration)
- 📦 **fnOS Native App**: Ships as a fnOS native app package with built-in WebUI and optional auto-mount

## 📋 System Requirements

- Go 1.21 or higher (for compilation)
- Supported OS: Windows, Linux, macOS
- Remote WebDAV storage service (e.g., Nextcloud, Nutstore, etc.) or S3-compatible storage (MinIO, Cloudflare R2, AWS S3, etc.)

## 🚀 Quick Start

### Method 1: Direct Execution (Recommended for Development/Testing)

1. **Clone Repository**
```bash
git clone https://github.com/vicnoah/clearvault.git
cd clearvault
```

2. **Build Project**
```bash
go build -o clearvault ./cmd/clearvault
```

3. **Create Configuration File**

Create `config.yaml`:
```yaml
server:
  # Listen address and port
  listen: "0.0.0.0:8080"
  # WebDAV base URL (default is /)
  base_url: "/dav"

  # Authentication credentials
  auth:
    user: "admin"
    pass: "your-secure-password"

security:
  # Master encryption key (32 bytes)
  # If left empty or default, a secure key will be auto-generated and saved here on first run
  master_key: "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY"

storage:
  # Metadata storage configuration (using local filesystem)
  metadata_path: "storage/metadata"
  cache_dir: "storage/cache"

remote:
  # Remote WebDAV storage configuration
  url: "https://your-webdav-server.com/remote.php/dav/files/username/"
  user: "your-webdav-username"
  pass: "your-webdav-password"
```

4. **Start Service**
```bash
./clearvault server --config config.yaml
```

After startup, the local WebDAV service address is: `http://127.0.0.1:8080/dav/`

### Method 2: Docker Deployment (Recommended for Production)

See [Docker Deployment](#-docker-deployment)

### Method 3: fnOS Native App

After installing the fnOS native app, you can complete initialization/configuration in WebUI and optionally enable auto FUSE mount. See [fnOS Native App](#-fnos-native-app).

## 📖 Usage Guide

### Windows Explorer

1. Open "This PC"
2. Right-click on empty space, select "Add a network location"
3. Enter address: `http://127.0.0.1:8080/dav/`
4. Enter username and password (configured in config.yaml)
5. Use it like a local disk after completion

### RaiDrive (Recommended)

1. Download and install [RaiDrive](https://www.raidrive.com/)
2. Click "Add" → Select "WebDAV"
3. Configure:
   - Address: `http://127.0.0.1:8080/dav/`
   - Username/Password: Authentication credentials from config.yaml
4. Click "Connect" to mount encrypted storage as a local disk

### macOS Finder

1. Open Finder
2. Menu bar: "Go" → "Connect to Server" (or press Cmd+K)
3. Enter address: `http://127.0.0.1:8080/dav/`
4. Enter username and password
5. Access after connection

### Linux (davfs2)

```bash
# Install davfs2
sudo apt-get install davfs2  # Debian/Ubuntu
sudo yum install davfs2       # CentOS/RHEL

# Create mount point
sudo mkdir -p /mnt/clearvault

# Mount
sudo mount -t davfs http://127.0.0.1:8080/dav/ /mnt/clearvault

# Enter username and password
```

### Offline Encrypted Export (Manual Cloud Upload)

In some environments, WebDAV uploads for large files may be unstable. You can first encrypt a batch of files locally into a single output directory, then manually upload that directory to the cloud using a browser, client, or offline tools.

1. Prepare configuration with a stable master key and metadata storage:
   - `security.master_key` must be stable and identical to the one used by your online ClearVault service
   - `storage.metadata_path` determines where metadata will be written

2. Run a one‑shot offline export command (it does not start the WebDAV server):

```bash
./clearvault encrypt --config config.yaml -in /path/to/plain-dir-or-file -out /path/to/export-dir
```

Parameter description:

- `-in`: Local path to export (single file or a directory)
- `-out`: Output directory for encrypted files; it will only contain ciphertext files with random names
- `--config`: Configuration file path (default "config.yaml")

Notes:

- After export, metadata for this batch of files is written under `storage.metadata_path`, including original paths and key info
- Filenames in `-out` are random `remoteName` values; you can upload all files in this directory to any folder in your target WebDAV storage
- As long as you later start ClearVault on the server with the same `config.yaml` (especially the same `master_key` and `metadata_path`), you can access these uploaded encrypted files via the WebDAV interface

## 🐳 Docker Deployment

### Using Docker Compose (Recommended)

1. **Create Configuration File**

Create `config.yaml` (refer to configuration example above)

2. **Start Service**
```bash
docker-compose up -d
```

3. **View Logs**
```bash
docker-compose logs -f
```

4. **Stop Service**
```bash
docker-compose down
```

### Using Docker Commands

```bash
# Run published image (recommended)
docker run -d \
  --name clearvault \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/storage:/app/storage \
  ghcr.io/vicnoah/clearvault:latest

# Build image locally if needed
docker build -t clearvault:latest .
```

### Docker Environment Variable Configuration

Besides using a configuration file, you can also configure via environment variables:

```bash
# Option A: Start with a configuration file
docker run -d \
  --name clearvault \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/storage:/app/storage \
  ghcr.io/vicnoah/clearvault:latest

# Option B: Start entirely with environment variables (no config file needed)
# Command to generate a random key: openssl rand -base64 32
docker run -d \
  --name clearvault \
  -p 8080:8080 \
  -e MASTER_KEY="your-generated-base64-key" \
  -e SERVER_LISTEN="0.0.0.0:8080" \
  -e SERVER_AUTH_USER="admin" \
  -e SERVER_AUTH_PASS="your-password" \
  -e REMOTE_URL="https://your-webdav.com/dav/" \
  -e REMOTE_USER="user" \
  -e REMOTE_PASS="pass" \
  -v $(pwd)/storage:/app/storage \
  ghcr.io/vicnoah/clearvault:latest
```

Supported Environment Variables (can override config.yaml or be used as primary configuration):
- `MASTER_KEY` (Required if starting without a config file)
- `SERVER_LISTEN`
- `SERVER_BASE_URL`
- `SERVER_AUTH_USER`
- `SERVER_AUTH_PASS`
- `STORAGE_METADATA_PATH`
- `STORAGE_CACHE_DIR`
- `REMOTE_URL`
- `REMOTE_USER`
- `REMOTE_PASS`

**Notes**:
1. If you are not using a `config.yaml` file, you MUST provide the `MASTER_KEY` environment variable; otherwise, the application will exit with an error.
2. If `config.yaml` is present, environment variables will override the corresponding settings in the file.

### Docker + FUSE Mount (Optional)

If you want to run `clearvault mount` (FUSE mount) inside Docker, build with `Dockerfile.fuse` and run the container with the required FUSE permissions. See [TECHNICAL.md](TECHNICAL.md) for more details.

Example:

```bash
docker build -f Dockerfile.fuse -t clearvault:fuse .

docker run --rm -it \
  --device /dev/fuse \
  --cap-add SYS_ADMIN \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v /your/mountpoint:/mnt/clearvault:rshared \
  clearvault:fuse mount --config /app/config.yaml --mountpoint /mnt/clearvault
```

## 🧰 fnOS Native App

ClearVault ships as a fnOS native app (FPK), designed for NAS scenarios and easier setup:

- **Built-in WebUI**: Complete initialization and configuration (master key, remote storage, access token, etc.).
- **Uninitialized Safety**: WebDAV is not served until initialization is completed, avoiding startup failures caused by missing configs.
- **Optional Auto Mount**: Supports delayed FUSE auto-mount, and unmounts on app stop.

### Data Directories & Key Files

On fnOS, ClearVault uses `${TRIM_PKGVAR}` as the persistent directory (common example: `/vol1/@appdata/ClearVault.Native.App/`). Key paths:

- `${TRIM_PKGVAR}/config.yaml`: configuration file
- `${TRIM_PKGVAR}/metadata/`: metadata directory
- `${TRIM_PKGVAR}/cache/`: cache directory
- `${TRIM_PKGVAR}/info.log`: runtime log
- `${TRIM_PKGVAR}/app.pid`: main process PID
- `${TRIM_PKGVAR}/mount.config.json`: auto-mount config (`auto/mountpoint/delaySeconds`)
- `${TRIM_PKGVAR}/mount.pid`: mount process PID
- `${TRIM_PKGVAR}/mount.json`: current mount info (pid, mountpoint)

### Compatibility & Troubleshooting

For fnOS-specific FUSE behaviors (temporary files, rename timing, etc.), see: [docs/fnos-fuse-upload-behavior.md](docs/fnos-fuse-upload-behavior.md).

## 🧩 FUSE Mount

ClearVault can mount encrypted storage as a local directory via FUSE.

```bash
./clearvault mount --config config.yaml --mountpoint /path/to/mount
```

Notes:

- `--mountpoint` must be an existing directory.
- If you build the binary yourself and need FUSE support, see build/runtime notes in [TECHNICAL.md](TECHNICAL.md).


## 🔧 Configuration Guide


### Metadata Storage


**Pros:**
- Simple, no dependencies, easy to backup
- File-level isolation, avoiding database corruption risks
- Easy to manually view and edit

**Use Cases:**
- Personal use, file count < 10,000
- Need simple and reliable storage solution
- Avoid database dependencies

### Security Recommendations

1. **Master Key (master_key)**:
   - Use at least 32 bytes of strong random password (**Recommend leaving empty for auto-generation**)
   - Keep it safe; data cannot be recovered if lost
   - The auto-generated key will be saved to config.yaml; please backup this file
   - Master key is used to encrypt file encryption keys (FEK), core to data security

2. **Authentication Password**:
   - Use strong password
   - Change regularly
   - Should not be the same as master key

3. **Network Security**:
   - Use HTTPS in production (via reverse proxy like Nginx)
   - Do not expose service directly to the internet
   - Use VPN or SSH tunnel for access

## 🛠️ Simple Share Feature

ClearVault supports sharing metadata through password-encrypted tar packages, allowing direct file transfer. simple sharing offers the following core advantages:

### 🌟 Core Advantage: Separation of Metadata and Encrypted Data

**Instant Sharing**: Encrypted files in the cloud can utilize official sharing features of various cloud storage services (such as Aliyun Drive, Baidu Netdisk, Dropbox, etc.), typically achieving instant sharing without waiting for WebDAV uploads.

**No WebDAV Server Required**: Directly generate encrypted metadata tar packages without relying on remote WebDAV services, avoiding network instability issues.

**Maximize Cloud Storage Features**: Leverage cloud storage's high-speed uploads, instant transfer, sharing links, and other features to significantly improve sharing efficiency.

**Separate Design**: Metadata sharing packages and encrypted files are completely separated. Metadata can be transmitted offline, while encrypted files are shared through cloud storage, without interfering with each other.

**Absolute Security**: Multi-layer encryption using PBKDF2 + AES-256-GCM + RSA-2048 ensures data security.

**Zero-Configuration Sharing**: Generated metadata tar packages can be shared across any platform that supports file transfer (email, cloud storage, instant messaging, etc.).

**Fully Offline**: Complete encrypted sharing without internet connection, perfect for sensitive data transmission.

**Use-and-Discard**: Each share generates an independent temporary key pair, avoiding key reuse risks.

### Export Share Package

```bash
# Export with specified password
./clearvault export \
    --paths "/documents/report.pdf" \
    --output /tmp/export \
    --share-key "my-secret-password"

# Auto-generate random password (16 characters)
./clearvault export \
    --paths "/documents/report.pdf" \
    --output /tmp/export

# Use specified configuration file
./clearvault export \
    --config config-s3.yaml \
    --paths "/documents" \
    --output /tmp/export
```

### Import Share Package

```bash
./clearvault import \
    --input /tmp/share_abc123.tar \
    --share-key "my-secret-password"

# Use specified configuration file
./clearvault import \
    --config config-s3.yaml \
    --input /tmp/share.tar \
    --share-key "password"
```

### Share Package Structure

```
share_abc123.tar
├── manifest.json          # Manifest file
├── metadata/              # Encrypted metadata files
│   └── timestamp_random_hash_filename.enc
└── private_key.enc        # Encrypted temporary private key
```

### Security Features

- **PBKDF2**: 100,000 key derivation iterations to prevent brute force attacks
- **AES-256-GCM**: Metadata encryption with authenticated encryption
- **RSA-2048**: Temporary key encryption with asymmetric protection
- **Temporary Keys**: New key pair generated for each share to avoid key reuse
- **Random Passwords**: Auto-generated 16-character random passwords for enhanced security
- **Random Filenames**: Timestamp + random + path hash ensures absolute collision-free

### Use Cases

1. **Instant Cloud Sharing**: Utilize cloud storage official sharing features for instant encrypted file sharing
2. **Sensitive Data Sharing**: Transmit sensitive files over insecure networks
3. **Offline Work**: Complete data sharing without network connection
4. **Cross-Platform Sharing**: Share encrypted data between Windows, Linux, and macOS
5. **Backup & Archive**: Backup encrypted metadata to local or other storage
6. **Temporary Sharing**: Quickly generate encrypted packages for temporary sharing without WebDAV setup

### Sharing Workflow Example

```
1. Export metadata sharing package locally
   ↓
2. Upload encrypted files to cloud (using cloud instant transfer)
   ↓
3. Share encrypted file link via cloud platform
   ↓
4. Send metadata sharing package through other channels
   ↓
5. Recipient imports metadata sharing package
   ↓
6. Recipient downloads encrypted files via share link
   ↓
7. Recipient decrypts and accesses original files
```

## 🛠️ Advanced Features

### Command Line Help

ClearVault provides a comprehensive command line help system:

```bash
# View all available commands
./clearvault --help

# View help for specific commands
./clearvault encrypt --help
./clearvault export --help
./clearvault import --help
./clearvault server --help
```

### Reverse Proxy Configuration (Nginx)

```nginx
server {
    listen 443 ssl http2;
    server_name vault.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location /dav/ {
        proxy_pass http://127.0.0.1:8080/dav/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebDAV special configuration
        client_max_body_size 0;
        proxy_request_buffering off;
    }
}
```

### Performance Optimization

1. **Remote Storage**: Choose WebDAV service with low network latency
2. **Local Cache**: Consider adding a caching layer at frontend (e.g., nginx cache)
3. **Large File Handling**: Streaming encryption/decryption with low memory usage

## 📊 Technical Implementation

For detailed technical implementation documentation, see: [TECHNICAL.md](TECHNICAL.md)

Key Technical Features:
- AES-256-GCM encryption algorithm
- Filename randomization (SHA-256 hash)
- Streaming encryption/decryption
- Complete WebDAV protocol implementation
- Windows file locking handling
- RaiDrive client compatibility optimization
- Simple share feature with multi-layer encryption

## 🤝 Contributing

Issues and Pull Requests are welcome!

## 📄 License

MIT License

## ⚠️ Disclaimer

This project is for learning and research purposes only. When using this software:
- Ensure compliance with local laws and regulations
- Keep the master key safe; data cannot be recovered if lost
- The author is not responsible for data loss or security issues
- Regular backups of important data are recommended

## 📮 Contact

For questions or suggestions, please contact via GitHub Issues.

---

**Note**: This documentation was updated to reflect the command line changes in v1.2.0. See [CHANGELOG.md](CHANGELOG.md) for detailed migration guide.
