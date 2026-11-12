#!/bin/bash

# Build script for git-works containers
# Supports building the standard app image, VSCode/Podman image, and OpenCode image

set -euo pipefail

# Configuration
REGISTRY="ghcr.io/mwildt"
IMAGE_NAME="git-works"
VSCODE_IMAGE_NAME="git-works-vscode"
OPENCODE_IMAGE_NAME="git-works-opencode"
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

usage() {
    cat <<EOF
Usage: $0 [OPTIONS] COMMAND

Build script for git-works containers.

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
  -r, --registry REGISTRY    Set the container registry (default: $REGISTRY)
  -t, --tag TAG              Set a custom tag (default: latest)
  --no-cache                Build without cache
  --load                    Load from build cache
  --platform PLATFORMS      Set target platforms (default: linux/amd64,linux/arm64)
  -h, --help                 Show this help message

Examples:
  $0 build                    # Build all images
  $0 build:app                # Build only the app image
  $0 build:vscode             # Build only the VSCode image
  $0 build:opencode           # Build only the OpenCode image
  $0 push                     # Push all images
  $0 -t v1.0.0 build          # Build with custom tag
  $0 --platform linux/amd64 build  # Build for specific platform
  $0 run                      # Run VSCode container locally
  $0 run:opencode             # Run OpenCode container locally
EOF
}

# Parse arguments
TAG="latest"
PLATFORMS="linux/amd64,linux/arm64"
NO_CACHE=false
LOAD_CACHE=false
COMMAND=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -r|--registry)
            REGISTRY="$2"
            shift 2
            ;;
        -t|--tag)
            TAG="$2"
            shift 2
            ;;
        --no-cache)
            NO_CACHE=true
            shift
            ;;
        --load)
            LOAD_CACHE=true
            shift
            ;;
        --platform)
            PLATFORMS="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            if [[ -z "$COMMAND" ]]; then
                COMMAND="$1"
            else
                COMMAND="$COMMAND $1"
            fi
            shift
            ;;
    esac
done

# Build arguments
BUILD_ARGS=(
    "--build-arg" "BUILD_DATE=$BUILD_DATE"
    "--build-arg" "GIT_COMMIT=$GIT_COMMIT"
    "--build-arg" "GIT_BRANCH=$GIT_BRANCH"
)

if [[ "$NO_CACHE" == true ]]; then
    BUILD_ARGS+=("--no-cache")
fi

if [[ "$LOAD_CACHE" == true ]]; then
    BUILD_ARGS+=("--cache-from" "type=gha")
fi

# Platform handling
IFS=',' read -ra PLATFORM_ARRAY <<< "$PLATFORMS"
PLATFORM_ARGS=()
for platform in "${PLATFORM_ARRAY[@]}"; do
    PLATFORM_ARGS+=("--platform" "$platform")
done

# Get dockerfile path for a given image name
get_dockerfile() {
    local image_name="$1"
    case "$image_name" in
        "$VSCODE_IMAGE_NAME")
            echo "podman/vscode/Containerfile"
            ;;
        "$OPENCODE_IMAGE_NAME")
            echo "podman/opencode/Containerfile"
            ;;
        *)
            echo "Containerfile"
            ;;
    esac
}

# Docker build command helper
docker_build() {
    local dockerfile="$1"
    local image_name="$2"
    local full_image="${REGISTRY}/${image_name}:${TAG}"
    
    log_info "Building ${full_image} from ${dockerfile}"
    
    # For multi-platform builds, use buildx
    if [[ ${#PLATFORM_ARRAY[@]} -gt 1 ]]; then
        docker buildx build \
            "${PLATFORM_ARGS[@]}" \
            "${BUILD_ARGS[@]}" \
            -t "$full_image" \
            -f "$dockerfile" \
            .
    else
        docker build \
            "${PLATFORM_ARGS[@]}" \
            "${BUILD_ARGS[@]}" \
            -t "$full_image" \
            -f "$dockerfile" \
            .
    fi
}

# Build the application image
build_app() {
    docker_build "Containerfile" "$IMAGE_NAME"
    log_info "Successfully built ${REGISTRY}/${IMAGE_NAME}:${TAG}"
}

# Build the VSCode image
build_vscode() {
    # First ensure the app is built (we need the builder stage)
    if ! docker image inspect "${REGISTRY}/${IMAGE_NAME}:${TAG}" &>/dev/null; then
        log_info "Building app image first (required for VSCode image)"
        build_app
    fi
    
    docker_build "podman/vscode/Containerfile" "$VSCODE_IMAGE_NAME"
    log_info "Successfully built ${REGISTRY}/${VSCODE_IMAGE_NAME}:${TAG}"
}

# Build the OpenCode image
build_opencode() {
    docker_build "podman/opencode/Containerfile" "$OPENCODE_IMAGE_NAME"
    log_info "Successfully built ${REGISTRY}/${OPENCODE_IMAGE_NAME}:${TAG}"
}

# Push images
push_images() {
    local images=("$@")
    
    for image in "${images[@]}"; do
        local full_image="${REGISTRY}/${image}:${TAG}"
        log_info "Pushing ${full_image}"
        
        if [[ ${#PLATFORM_ARRAY[@]} -gt 1 ]]; then
            local dockerfile
            dockerfile=$(get_dockerfile "$image")
            docker buildx build \
                "${PLATFORM_ARGS[@]}" \
                -t "$full_image" \
                --push \
                -f "$dockerfile" \
                .
        else
            docker push "$full_image"
        fi
    done
}

# Clean images
clean_images() {
    local images=("$@")
    
    for image in "${images[@]}"; do
        local full_image="${REGISTRY}/${image}:${TAG}"
        if docker image inspect "$full_image" &>/dev/null; then
            log_info "Removing ${full_image}"
            docker rmi "$full_image"
        fi
    done
}

# Run VSCode container
run_vscode() {
    local container_name="git-works-vscode"
    local port=8080
    local data_dir="/tmp/git-works-data"
    
    # Create data directory if it doesn't exist
    mkdir -p "$data_dir"
    
    log_info "Starting VSCode container on port ${port}"
    log_info "Data directory: ${data_dir}"
    log_info "Access VSCode at: http://localhost:${port}"
    log_warn "Default password: change-me-in-production (change in podman/vscode/Containerfile)"
    
    docker run -it --rm \
        --name "$container_name" \
        -p "$port:$port" \
        -v "$data_dir:/data" \
        -v "$(pwd)/.git:/app/.git:ro" \
        "${REGISTRY}/${VSCODE_IMAGE_NAME}:${TAG}"
}

# Run OpenCode container
run_opencode() {
    local container_name="git-works-opencode"
    local port=4096
    local workspace_dir="/tmp/git-works-opencode"
    
    # Create workspace directory if it doesn't exist
    mkdir -p "$workspace_dir"
    
    log_info "Starting OpenCode container on port ${port}"
    log_info "Workspace directory: ${workspace_dir}"
    log_info "Access OpenCode at: http://localhost:${port}"
    
    docker run -it --rm \
        --name "$container_name" \
        -p "$port:$port" \
        -v "$workspace_dir:/workspace" \
        -v "$(pwd):/workspace/project:ro" \
        "${REGISTRY}/${OPENCODE_IMAGE_NAME}:${TAG}"
}

# Main command handling
case "$COMMAND" in
    build)
        log_info "Building all images with tag: ${TAG}"
        build_app
        build_vscode
        build_opencode
        ;;
    build:app)
        build_app
        ;;
    build:vscode)
        build_vscode
        ;;
    build:opencode)
        build_opencode
        ;;
    push)
        log_info "Pushing all images with tag: ${TAG}"
        push_images "$IMAGE_NAME" "$VSCODE_IMAGE_NAME" "$OPENCODE_IMAGE_NAME"
        ;;
    push:app)
        push_images "$IMAGE_NAME"
        ;;
    push:vscode)
        push_images "$VSCODE_IMAGE_NAME"
        ;;
    push:opencode)
        push_images "$OPENCODE_IMAGE_NAME"
        ;;
    clean)
        log_info "Cleaning all images with tag: ${TAG}"
        clean_images "$IMAGE_NAME" "$VSCODE_IMAGE_NAME" "$OPENCODE_IMAGE_NAME"
        ;;
    run)
        run_vscode
        ;;
    run:opencode)
        run_opencode
        ;;
    "")
        usage
        exit 1
        ;;
    *)
        log_error "Unknown command: $COMMAND"
        usage
        exit 1
        ;;
esac

log_info "Done!"
