#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/../.env"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

if [[ ! -f "$ENV_FILE" ]]; then
    echo -e "${RED}Error: .env file not found at $ENV_FILE${NC}"
    exit 1
fi

# Load .env (skip comments and empty lines)
while IFS='=' read -r key value; do
    # Skip comments and empty lines
    [[ "$key" =~ ^[[:space:]]*# ]] && continue
    [[ -z "$key" ]] && continue
    # Trim whitespace and quotes
    key=$(echo "$key" | xargs)
    value=$(echo "$value" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' | sed 's/^["\'"'"']//;s/["\'"'"']$//')
    export "$key=$value"
done < "$ENV_FILE"

# Validate required vars
MISSING=0
for var in DEPLOY_HOST DEPLOY_USER DEPLOY_PASS; do
    if [[ -z "${!var:-}" ]]; then
        echo -e "${RED}Error: $var is not set in .env${NC}"
        MISSING=1
    fi
done

if [[ $MISSING -eq 1 ]]; then
    exit 1
fi

echo -e "${YELLOW}Testing SSH connection...${NC}"
echo "  Host: $DEPLOY_HOST"
echo "  User: $DEPLOY_USER"
echo ""

if ! command -v sshpass &> /dev/null; then
    echo -e "${RED}Error: sshpass is not installed.${NC}"
    echo "Install it with: brew install sshpass   (macOS)"
    echo "            or: sudo apt install sshpass (Ubuntu)"
    exit 1
fi

if ! command -v ssh &> /dev/null; then
    echo -e "${RED}Error: ssh is not installed.${NC}"
    exit 1
fi

# Test connection (10 second timeout)
if sshpass -p "$DEPLOY_PASS" ssh -o StrictHostKeyChecking=no \
    -o ConnectTimeout=10 \
    -o BatchMode=no \
    "${DEPLOY_USER}@${DEPLOY_HOST}" "echo 'SSH connection successful'" 2>/dev/null; then
    echo ""
    echo -e "${GREEN}✓ SSH connection successful${NC}"
    exit 0
else
    echo ""
    echo -e "${RED}✗ SSH connection failed${NC}"
    echo "Possible causes:"
    echo "  - Invalid credentials"
    echo "  - Host unreachable"
    echo "  - SSH port blocked"
    echo "  - Password authentication disabled on server"
    exit 1
fi
