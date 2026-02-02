.PHONY: help setup dev stop restart clean prune logs build-server build-webapp restart-server restart-webapp

# Default target
help:
	@echo "Mattermost Development Commands:"
	@echo ""
	@echo "  make setup           - Initial setup (install deps, build everything)"
	@echo "  make dev             - Start development (server + postgres + webapp)"
	@echo "  make stop            - Stop all services"
	@echo "  make restart         - Restart all services"
	@echo ""
	@echo "  make build-server    - Build only server Docker image"
	@echo "  make restart-server  - Rebuild and restart server"
	@echo "  make logs-server     - View server logs"
	@echo ""
	@echo "  make build-webapp    - Build webapp"
	@echo "  make restart-webapp  - Restart webapp dev server"
	@echo "  make install-webapp  - Install webapp dependencies"
	@echo ""
	@echo "  make logs            - View all logs"
	@echo "  make clean           - Stop and remove all containers/volumes"
	@echo "  make prune           - Clean Docker system (removes unused images/cache)"
	@echo "  make ps              - Show running containers"

# Initial setup
setup:
	@echo "Setting up Mattermost..."
	@cp -n .env.example .env 2>/dev/null || true
	@echo "Cleaning previous build..."
	@docker compose down -v 2>/dev/null || true
	@echo "Installing webapp dependencies..."
	@cd webapp && npm i
	@echo "Building webapp..."
	@cd webapp && npm run build
	@echo "Building server (this may take a few minutes)..."
	@docker compose build --no-cache
	@echo ""
	@echo "✓ Setup complete! Run 'make dev' to begin."

# Development mode (server + webapp dev)
dev:
	@echo "Starting Mattermost in development mode..."
	@docker compose up -d
	@echo "Waiting for server..."
	@sleep 5
	@echo ""
	@echo "✓ Server running at: http://localhost:8065"
	@echo "✓ Starting webapp dev server at: http://localhost:9005"
	@echo ""
	@cd webapp && npm run dev-server

# Stop all services
stop:
	@echo "Stopping Mattermost..."
	@docker compose down
	@pkill -f "npm run dev-server" 2>/dev/null || true
	@echo "✓ All services stopped"

# Restart all services
restart: stop dev

# Build server only
build-server:
	@echo "Building server..."
	@docker compose build mattermost-server

# Restart server only
restart-server:
	@echo "Rebuilding and restarting server..."
	@docker compose up -d --build mattermost-server
	@echo "✓ Server restarted"

# Build webapp
build-webapp:
	@echo "Building webapp..."
	@cd webapp && npm run build
	@echo "✓ Webapp built"

# Restart webapp dev server
restart-webapp:
	@echo "Restarting webapp dev server..."
	@pkill -f "npm run dev-server" 2>/dev/null || true
	@sleep 1
	@cd webapp && npm run dev-server

# Install webapp dependencies
install-webapp:
	@echo "Installing webapp dependencies..."
	@cd webapp && npm i
	@echo "✓ Dependencies installed"

# View all logs
logs:
	@docker compose logs -f

# View server logs only
logs-server:
	@docker compose logs -f mattermost-server

# View postgres logs
logs-postgres:
	@docker compose logs -f postgres

# Show running containers
ps:
	@docker compose ps

# Clean everything
clean:
	@echo "Cleaning up..."
	@docker compose down -v
	@pkill -f "npm run dev-server" 2>/dev/null || true
	@echo "✓ Cleaned up (containers, volumes, and processes removed)"

# Prune Docker system
prune:
	@echo "Pruning Docker system..."
	@docker system prune -af --volumes
	@echo "✓ Docker system pruned (removed unused images, containers, volumes, and cache)"

# Deep clean (including node_modules)
clean-all: clean
	@echo "Deep cleaning..."
	@rm -rf webapp/node_modules
	@rm -rf webapp/dist
	@echo "✓ Deep clean complete"
