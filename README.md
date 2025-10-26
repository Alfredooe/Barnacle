<img width="2048" height="512" alt="image" src="https://github.com/user-attachments/assets/6fac9ffc-3928-4f78-9b46-1fb9eea3bfc1" />

I got annoyed with having to use Portainer / Komodo for continuously deploying docker compose stacks from Github. Barnacle is a simple Golang tool built deployed as a container that polls a git repo and deploys docker compose stacks found in subfolders. It's a bit like Flux for Kube, but simpler and sticks onto a docker host to deploy your stacks. Hence, Barnacle. 

## Features

- Minimal and understandable
- Polls a git repo using a deploy key every 30s
- Clones repo to `/opt/reponame`
- Detects compose stack changes then runs a compose up / down to deploy the changed stack.
- Stacks can be ignored using a `ignore` file to not operate on them.
- Can webhook updates and deployment status to Slack and Discord.

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

Run the project with docker compose. This can be from your stacks repo, but I'd recommend adding an ignore flag on Barnacle itself.

## Repository Structure

Your repo should be structured like this. Add an ignore file to ignore a stack.

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

## Security Notes

- Container needs access to the hosts docker socket to manage containers.
- Container needs a mount to /opt to store the repo

## Logging

This exists, It's a little bit verbose and needs improving. 
