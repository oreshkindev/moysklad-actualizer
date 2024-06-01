#!/bin/sh

. ./env.sh

# This script is used to deploy the binary file to a server and
# restart the corresponding service.
#
# Args:
#   None

# Build the binary
GOOS=linux GOARCH=amd64 go build -o ${LOCAL_PATH} *.go

# Deploy the binary file from local machine to the server
rsync --archive --compress --delete ${LOCAL_PATH} ${SSH_USER}@${SSH_HOST}:${REMOTE_PATH}

# Restart the service on the server
ssh "${SSH_USER}@${SSH_HOST}" "
    sudo systemctl restart ${SERVICE_NAME}
"

# Inform the user that the script has finished executing
if [ $? -eq 0 ]; then
    echo "Binary has been deployed successfully."
else
    echo "Binary deployment failed."
    exit 1
fi
