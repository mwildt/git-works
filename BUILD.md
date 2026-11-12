# Build Script Documentation

This directory contains build scripts for creating container images for git-works.

## Available Images

### Standard Application Image (`Containerfile`)
- Based on: `alpine:3.20`
- Contains: The compiled git-works application
- Use case: Production deployment

### VSCode/Podman Image (`podman/vscode/Containerfile`)
- Based on: `codercom/code-server:latest`
- Contains: code-server + git-works application
- Use case: Development with VSCode in browser
- Features:
  - Full VSCode IDE in browser
  - Integrated git-works functionality
  - Persistent workspace in `/data`

### OpenCode Image (`podman/opencode/Containerfile`)
- Based on: `node:24-trixie`
- Contains: OpenCode CLI
- Use case: AI-powered code development
- Features:
  - OpenCode IDE in browser
  - Port: 4096
  - Workspace in `/workspace`

## Build Script Usage

### Quick Start

```bash
# Build all images
./build.sh build

# Build only the VSCode image
./build.sh build:vscode

# Run VSCode container locally
./build.sh run
```

### Full Command Reference

```bash
./build.sh [OPTIONS] COMMAND

Commands:
  build              Build all container images
  build:app          Build the standard application image
  build:vscode       Build the VSCode/Podman image
  build:opencode     Build the OpenCode image
  push               Push all images to registry
  push:app           Push the standard application image
  push:vscode        Push the VSCode/Podman image
  push:opencode      Push the OpenCode image
  clean              Remove built images
  run                Run the VSCode container locally
  run:opencode       Run the OpenCode container locally

Options:
  -r, --registry REGISTRY    Set the container registry
  -t, --tag TAG              Set a custom tag
  --no-cache                Build without cache
  --load                    Load from build cache
  --platform PLATFORMS      Set target platforms
  -h, --help                 Show help
```

### Examples

```bash
# Build with custom tag
./build.sh -t v1.0.0 build

# Build for specific platform
./build.sh --platform linux/amd64 build

# Build and push to Docker Hub
./build.sh -r docker.io/yourusername build
./build.sh -r docker.io/yourusername push

# Build without cache
./build.sh --no-cache build

# Run VSCode container
./build.sh run

# Run OpenCode container
./build.sh run:opencode

# Build only OpenCode image
./build.sh build:opencode

# Push only OpenCode image
./build.sh push:opencode
```

## Environment Configuration

Create a `.env` file to override build defaults:

```bash
# Copy the template
cp .env.build .env

# Edit the file to set your configuration
nano .env
```

The build script will automatically load variables from `.env` if it exists.

## Multi-Platform Builds

The build script supports multi-platform builds using Docker Buildx:

```bash
# Build for both AMD64 and ARM64
./build.sh --platform linux/amd64,linux/arm64 build

# Push multi-platform images
./build.sh --platform linux/amd64,linux/arm64 push
```

## VSCode Container

### Running the VSCode Container

```bash
# Start the container
./build.sh run

# Or manually with docker:
docker run -it --rm \
    -p 8080:8080 \
    -v /path/to/data:/data \
    ghcr.io/mwildt/git-works-vscode:latest
```

### Accessing VSCode

After starting the container:
- Open your browser to: `http://localhost:8080`
- Password: `change-me-in-production` (change this in `podman/vscode/Containerfile`)

### Customizing VSCode

Edit `podman/vscode/Containerfile` to:
- Change the default password
- Add VSCode extensions
- Configure code-server settings

Example for adding extensions to `podman/vscode/Containerfile`:
```dockerfile
RUN code-server --install-extension ms-vscode.go \
    && code-server --install-extension redhat.vscode-yaml
```

## OpenCode Container

### Running the OpenCode Container

```bash
# Start the container
./build.sh run:opencode

# Or manually with docker:
docker run -it --rm \
    -p 4096:4096 \
    -v /path/to/workspace:/workspace \
    ghcr.io/mwildt/git-works-opencode:latest
```

### Accessing OpenCode

After starting the container:
- Open your browser to: `http://localhost:4096`
- The workspace is mounted from `/workspace`

## Podman Support

The images work with both Docker and Podman. For Podman:

```bash
# Build with Podman
podman build -t git-works-vscode -f podman/vscode/Containerfile .

# Run with Podman
podman run -it --rm -p 8080:8080 -v ./data:/data git-works-vscode
```

## CI/CD Integration

The existing GitHub Actions workflow (`.github/workflows/build-and-publish.yml`) can be extended to build the VSCode image:

```yaml
- name: Build VSCode image
  run: ./build.sh build:vscode

- name: Push VSCode image
  run: ./build.sh push:vscode
```

## Troubleshooting

### Build fails with cache issues
```bash
./build.sh --no-cache build
```

### Permission issues with volumes
Ensure the data directory has the correct permissions:
```bash
mkdir -p /path/to/data
chmod 777 /path/to/data  # For development only
```

### VSCode not accessible
Check if the container is running:
```bash
docker ps
```

Check the logs:
```bash
docker logs git-works-vscode
```

## Security Notes

1. **Password**: The default password in `podman/vscode/Containerfile` is `change-me-in-production`. Change this before deploying!
2. **User**: The container runs as a non-root user (`appuser`) for security.
3. **Port**: The container exposes port 8080 by default.
4. **Data**: Mount the `/data` volume for persistent storage.

## Image Tags

The build script automatically sets the following labels:
- `org.opencontainers.image.created`: Build date
- `org.opencontainers.image.revision`: Git commit
- `org.opencontainers.image.version`: Tag
