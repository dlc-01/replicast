#!/bin/bash
# Генерирует RSA-2048 ключи для каждого узла
# Использование: ./scripts/gen_node_keys.sh
# Ключи сохраняются в deployments/keys/

set -e

KEYS_DIR="deployments/keys"
mkdir -p "$KEYS_DIR"

for NODE in node-a node-b node-c; do
    KEY_FILE="$KEYS_DIR/${NODE}.pem"
    if [ -f "$KEY_FILE" ]; then
        echo "[$NODE] key already exists, skipping"
        continue
    fi
    openssl genrsa -out "$KEY_FILE" 2048 2>/dev/null
    chmod 600 "$KEY_FILE"
    echo "[$NODE] generated: $KEY_FILE"
done

echo ""
echo "Add to each node's .env:"
echo "  NODE_KEY_PATH=/app/keys/node-a.pem  (adjust node name)"