# Local Development Setup Guide

This guide will help you set up the Mattermost development environment with Taruvi integration on your local machine.

## Prerequisites

Before you begin, ensure you have the following installed:

- **Docker** (version 20.10 or higher)
- **Docker Compose** (version 2.0 or higher)
- **Git**

## Quick Setup

### 1. Clone the Repository

```bash
git clone <repository-url>
cd mattermost
```

### 2. Initial Setup

Run the setup command to build and configure the development environment:

```bash
make setup
```

This command will:
- Copy the example environment file
- Build the server Docker image
- Install webapp dependencies
- Build the webapp

### 3. Start Development Environment

Start the development servers:

```bash
make dev
```

This will:
- Start the PostgreSQL database
- Start the Mattermost server
- Start the webapp development server

The application will be available at:
- **Mattermost Server**: http://localhost:8065
- **Webapp Dev Server**: http://localhost:9005

## Configuration

### Environment Variables

The setup uses environment variables defined in the `.env` file. The `.env.example` file contains default values that will be copied during setup.

Key configuration options:

```bash
# PostgreSQL Settings
POSTGRES_USER=mmuser
POSTGRES_PASSWORD=mostest
POSTGRES_DB=mattermost
POSTGRES_PORT=5435

# Mattermost Server Settings
MATTERMOST_PORT=8065
MM_SERVICESETTINGS_SITEURL=http://localhost:8065
MM_LOGSETTINGS_CONSOLELEVEL=INFO

# Database Connection
MM_SQLSETTINGS_DATASOURCE=postgres://mmuser:mostest@postgres:5432/mattermost?sslmode=disable

# Taruvi Authentication Settings
MM_TARUVISETTINGS_ENABLE=true
MM_TARUVISETTINGS_TARUVISERVERURL=http://taruvi_web:8000/sites/appbuild
MM_TARUVISETTINGS_AUTHENDPOINT=/_allauth/app/v1/auth/login
MM_TARUVISETTINGS_USERENDPOINT=/api/users/me/
MM_TARUVISETTINGS_CONNECTIONTIMEOUT=10
MM_TARUVISETTINGS_OVERRIDEHOST=false
MM_TARUVISETTINGS_HOSTOVERRIDEVALUE=localhost:8000
```

### Customizing Taruvi Integration

To customize the Taruvi integration, modify the following environment variables:

- `MM_TARUVISETTINGS_ENABLE`: Enable/disable Taruvi authentication (default: true)
- `MM_TARUVISETTINGS_TARUVISERVERURL`: Base URL for the Taruvi server
- `MM_TARUVISETTINGS_AUTHENDPOINT`: Authentication endpoint path
- `MM_TARUVISETTINGS_USERENDPOINT`: User info endpoint path
- `MM_TARUVISETTINGS_CONNECTIONTIMEOUT`: Connection timeout in seconds
- `MM_TARUVISETTINGS_OVERRIDEHOST`: Enable/disable Host header override (default: false)
- `MM_TARUVISETTINGS_HOSTOVERRIDEVALUE`: Value for Host header when override is enabled

## Development Commands

The project includes several useful make commands:

```bash
# View available commands
make help

# Start development environment
make dev

# Stop all services
make stop

# Restart all services
make restart

# Build only the server
make build-server

# Build only the webapp
make build-webapp

# Restart only the server
make restart-server

# Restart only the webapp
make restart-webapp

# View all logs
make logs

# View only server logs
make logs-server

# Clean up containers and volumes
make clean

# Clean Docker system (removes unused images/cache)
make prune
```

## Troubleshooting

### Common Issues

#### 1. Port Already in Use
If you get port binding errors, check if PostgreSQL or Mattermost is already running:
```bash
# Check running processes
docker ps
# Kill conflicting containers if needed
docker kill <container-name>
```

#### 2. Database Connection Issues
If the server can't connect to the database:
```bash
# Check if PostgreSQL is running
make logs | grep postgres
# Restart the environment
make restart
```

#### 3. Build Failures
If the build fails:
```bash
# Clean and rebuild
make clean
make setup
```

#### 4. Webapp Not Loading
If the webapp doesn't load properly:
```bash
# Restart only the webapp
make restart-webapp
```

### Useful Commands

```bash
# Check running containers
make ps

# View server logs in real-time
make logs-server

# Install webapp dependencies separately
make install-webapp
```

## Development Workflow

### Making Changes

1. **Server Changes**: Modify files in the `server/` directory. Rebuild with `make build-server` and restart with `make restart-server`.

2. **Webapp Changes**: Modify files in the `webapp/` directory. The development server supports hot reloading.

3. **Configuration Changes**: Update the `.env` file and restart the services with `make restart`.

### Testing

The application will be accessible at http://localhost:8065. You can create accounts and test the Taruvi authentication integration.

## Stopping Development

To stop the development environment:

```bash
make stop
```

To completely clean up:

```bash
make clean
```

## Notes

- The first setup may take several minutes as it downloads dependencies and builds the Docker images
- The webapp development server runs on port 9005 but connects to the Mattermost server at port 8065
- All services run in Docker containers managed by Docker Compose
- The database data persists between restarts but is removed with `make clean`