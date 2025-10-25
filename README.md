# Barnacle

I got annoyed with having to use Portainer / Komodo for this. Simple Golang tool built into a container that polls a git repo and deploys docker compose stacks found in subfolders. A bit like Flux for compose stacks. Small and primitive, hence why it's called Barnacle.

## Features

- Polls a git repo using a deploy key every 30s
- Clones repo to `/opt/reponame`
- Detects compose stack changes and ups or deletes these
- Can ignore stacks with an `ignore` file
- Discord / Slack webhooks

## Quick Start

### 1. Deploy Key Setup

Generate an SSH key pair and add this to your repo
```bash
ssh-keygen -t ed25519 -C "deploy-key" -f deploy_key
```

### 2. Configure docker-compose.yml

Edit the `docker-compose.yml` file

```yaml
environment:
  - REPO_URL=git@github.com:youruser/yourrepo.git  # Your repo
  - DEPLOY_KEY_PATH=/ssh/deploy_key # This is internal to the container, Update the volume mount not this
  - BRANCH=main  # Desired branch
  - DISCORD_WEBHOOK=https://discord.com/api/webhooks/YOUR_WEBHOOK_URL  # Optional
```

Update the SSH key path in volumes if needed:
```yaml
volumes:
  - ./deploy_key:/ssh/deploy_key:ro  # Path to your deploy key
```

### 3. Run with Docker Compose

Docker compose up it.

## Repository Structure

Repo should be structured like this. Add an ignore file to ignore a stack.

```
your-repo/
├── stack1/
│   └── docker-compose.yml
├── stack2/
│   ├── docker-compose.yml
│   └── ignore                    # This stack will be skipped
└── stack3/
    └── compose.yml
```

## Sec Notes

- Container needs access to the hosts docker socket to manage containers.
- Container needs a mount to /opt to store the repo

## Logging

This exists, not much to say about it. 
