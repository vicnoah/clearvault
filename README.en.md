# ClearVault

English | [简体中文](README.md)

ClearVault is an encrypted cloud storage proxy service based on the WebDAV protocol. It encrypts files before storing them on any WebDAV-compatible cloud storage service (such as Nextcloud, Nutstore, etc.) while providing a local WebDAV interface for client access.

## ✨ Key Features

- 🔐 **End-to-End Encryption**: AES-256-GCM encryption algorithm with user-controlled master key
- 🌐 **WebDAV Protocol**: Compatible with all WebDAV clients (RaiDrive, Windows Explorer, macOS Finder, etc.)
- 📁 **Filename Encryption**: Complete encryption of filenames and directory structure; remote storage only saves random hashes
- 🚀 **Streaming Encryption/Decryption**: Supports streaming processing for large files with low memory usage
- 💾 **Flexible Metadata Storage**: Supports local filesystem or SQLite database for metadata storage
- 🔄 **Full WebDAV Support**: Supports file upload, download, delete, rename, directory operations, etc.
- 🪟 **Windows Optimization**: Special optimizations for Windows file locking and RaiDrive client

## 📋 System Requirements

- Go 1.21 or higher (for compilation)
- Supported OS: Windows, Linux, macOS
- Remote WebDAV storage service (e.g., Nextcloud, Nutstore, etc.)

## 🚀 Quick Start

### Method 1: Direct Execution (Recommended for Development/Testing)

1. **Clone Repository**
```bash
git clone https://github.com/yourusername/clearvault.git
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
  listen: "127.0.0.1:8080"
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
  # Metadata storage configuration
  metadata_type: "local"  # Options: local, sqlite
  metadata_path: "storage/metadata"  # Path for local type
  # db_path: "storage/metadata.db"  # Path for sqlite type
  cache_dir: "storage/cache"

remote:
  # Remote WebDAV storage configuration
  url: "https://your-webdav-server.com/remote.php/dav/files/username/"
  user: "your-webdav-username"
  pass: "your-webdav-password"
```

4. **Start Service**
```bash
./clearvault --config config.yaml
```

After startup, the local WebDAV service address is: `http://127.0.0.1:8080/dav/`

### Method 2: Docker Deployment (Recommended for Production)

See [Docker Deployment](#-docker-deployment)

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
   - `storage.metadata_type` / `storage.metadata_path` decide where metadata will be written

2. Run a one‑shot offline export command (it does not start the WebDAV server):

```bash
./clearvault -config config.yaml -in /path/to/plain-dir-or-file -out /path/to/export-dir
```

Parameter description:

- `-in`: Local path to export (single file or a directory)
- `-out`: Output directory for encrypted files; it will only contain ciphertext files with random names
- Legacy parameters (still supported for compatibility):
  - `-export-input` is equivalent to `-in` (when `-in` is not provided)
  - `-export-output` is equivalent to `-out` (when `-out` is not provided)

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
# Build image
docker build -t clearvault:latest .

# Run container
docker run -d \
  --name clearvault \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/storage:/app/storage \
  clearvault:latest
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
  clearvault:latest

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
  -e STORAGE_METADATA_TYPE="local" \
  -v $(pwd)/storage:/app/storage \
  clearvault:latest
```

Supported Environment Variables (can override config.yaml or be used as primary configuration):
- `MASTER_KEY` (Required if starting without a config file)
- `SERVER_LISTEN`
- `SERVER_BASE_URL`
- `SERVER_AUTH_USER`
- `SERVER_AUTH_PASS`
- `STORAGE_METADATA_TYPE`
- `STORAGE_METADATA_PATH`
- `STORAGE_CACHE_DIR`
- `REMOTE_URL`
- `REMOTE_USER`
- `REMOTE_PASS`

**Notes**:
1. If you are not using a `config.yaml` file, you MUST provide the `MASTER_KEY` environment variable; otherwise, the application will exit with an error.
2. If `config.yaml` is present, environment variables will override the corresponding settings in the file.


## 🔧 Configuration Guide

### Metadata Storage Types

**Local (Filesystem)**
- Pros: Simple, no dependencies, easy to backup
- Cons: Lower performance with many small files
- Use Case: Personal use, file count < 10,000

**SQLite (Database)**
- Pros: Better performance, supports large number of files
- Cons: Requires regular database backups
- Use Case: Large number of files, better performance needed

### Security Recommendations

1. **Master Key (master_key)**:
   - Use at least 32 bytes of strong random password (**Recommend leaving empty for auto-generation**)
   - Keep it safe; data cannot be recovered if lost
   - The auto-generated key will be saved to config.yaml; please backup this file

2. **Authentication Password**:
   - Use strong password
   - Change regularly
   - Should not be the same as master key

3. **Network Security**:
   - Use HTTPS in production (via reverse proxy like Nginx)
   - Do not expose service directly to the internet
   - Use VPN or SSH tunnel for access

## 🛠️ Advanced Features

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

1. **Metadata Storage**: Use SQLite when file count > 10,000
2. **Remote Storage**: Choose WebDAV service with low network latency
3. **Local Cache**: Consider adding a caching layer at frontend (e.g., nginx cache)

## 📊 Technical Implementation

For detailed technical implementation documentation, see: [TECHNICAL.md](TECHNICAL.md)

Key Technical Features:
- AES-256-GCM encryption algorithm
- Filename randomization (SHA-256 hash)
- Streaming encryption/decryption
- Complete WebDAV protocol implementation
- Windows file locking handling
- RaiDrive client compatibility optimization

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
