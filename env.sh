#!/bin/sh

export MOYSKLAD_TOKEN=""

# Database settings
#
# Format:
#   <dialect>://<username>:<password>@<host>:<port>/<database>
#
# Variables used in the URL:
#   dialect (str): The name of the database management system.
#   username (str): The username used to connect to the database.
#   password (str): The password used to connect to the database.
#   host (str): The hostname or IP address of the database server.
#   port (int): The port number on which the database server is listening.
#   database (str): The name of the database.

export DATABASE_URL="postgres://"

# Deployment settings
#
# Variables used in deploy.sh:
#   SSH_HOST (str): The IP address of the server to deploy to.
#   SSH_USER (str): The SSH user to connect with.
#   SERVICE_NAME (str): The name of the service to restart.
#   REMOTE_PATH (str): The path to the remote binary on the server.
#   LOCAL_PATH (str): The path to the local binary.

export SSH_HOST=""
export SSH_USER=""
#
export SERVICE_NAME="moysklad-actualize.service"
#
export REMOTE_PATH=""
#
export LOCAL_PATH="./bin/main"
