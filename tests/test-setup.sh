set -e
echo "Setting up test environment..."

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Check if test.env exists
if [ ! -f .env.test ]; then
    echo -e "${YELLOW}.env.test not found, creating from template...${NC}"
    cp .env.test.example .env.test
fi

# Start test services
echo "Starting test services..."
docker-compose -f docker/docker-compose.test.yml up -d

# Wait for services to be healthy
echo "Waiting for services..."
sleep 5

# Check PostgreSQL
until docker exec postgres-test pg_isready -U postgres > /dev/null 2>&1; do
    echo "Waiting for PostgreSQL..."
    sleep 2
done
echo -e "${GREEN}PostgreSQL ready${NC}"

# Check Redis
until docker exec redis-test redis-cli ping > /dev/null 2>&1; do
    echo "Waiting for Redis..."
    sleep 2
done
echo -e "${GREEN}Redis ready${NC}"

# Create test database
echo "Creating test database..."
docker exec postgres-test psql -U postgres -c "DROP DATABASE IF EXISTS securechat_test;" || true
docker exec postgres-test psql -U postgres -c "CREATE DATABASE securechat_test;"
echo -e "${GREEN}Database created${NC}"

# Run migrations
echo "Running migrations..."
export GOOSE_DRIVER="postgres"
export GOOSE_DBSTRING="postgres://postgres:postgres@localhost:5433/securechat_test?sslmode=disable"
cd sql/schema && goose up
cd ../..

echo -e "${GREEN}Migrations complete${NC}"
echo ""
echo -e "${GREEN}Test environment ready!${NC}"