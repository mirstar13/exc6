package setup

import (
	"context"
	"database/sql"
	"exc6/config"
	"exc6/db"
	infraredis "exc6/infrastructure/redis"
	"exc6/services/chat"
	"exc6/services/friends"
	"exc6/services/sessions"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"
)

type TestSuite struct {
	suite.Suite
	DB         *sql.DB
	Queries    *db.Queries
	Redis      *redis.Client
	ChatSvc    *chat.ChatService
	FriendSvc  *friends.FriendService
	SessionMgr *sessions.SessionManager
	Config     *config.Config
	Ctx        context.Context
}

func (s *TestSuite) SetupSuite() {
	// Initialize context first - this will be used throughout all tests
	s.Ctx = context.Background()

	// Load test environment
	if err := godotenv.Load("../../.env.test"); err != nil {
		log.Printf("No .env.test file found, using defaults")
	}

	// Load config
	cfg, err := config.Load()
	s.Require().NoError(err)
	s.Config = cfg

	// Setup test database
	s.setupTestDatabase()

	// Setup Redis
	s.setupRedis()

	// Setup services
	s.setupServices()
}

func (s *TestSuite) TearDownSuite() {
	// Cleanup
	if s.ChatSvc != nil {
		s.ChatSvc.Close()
	}
	if s.DB != nil {
		s.DB.Close()
	}
	if s.Redis != nil {
		s.Redis.Close()
	}
}

func (s *TestSuite) SetupTest() {
	// Clean database before each test
	// DON'T reset s.Ctx here - it breaks services that hold references to it
	s.cleanDatabase()
}

func (s *TestSuite) TearDownTest() {
	// Clean up after each test
	s.cleanDatabase()
	s.cleanRedis()
}

func (s *TestSuite) setupTestDatabase() {
	// Use test database
	testDBString := os.Getenv("GOOSE_DBSTRING")
	if testDBString == "" {
		testDBString = "postgres://postgres:postgres@localhost:5433/securechat_test?sslmode=disable"
	}

	pdb, err := sql.Open("postgres", testDBString)
	s.Require().NoError(err, "Failed to open database connection")

	// Verify connection
	err = pdb.Ping()
	s.Require().NoError(err, "Failed to ping database")

	s.DB = pdb
	s.Queries = db.New(pdb)
}

func (s *TestSuite) setupRedis() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6380"
	}

	rdb, err := infraredis.NewClient(config.RedisConfig{
		Address:  redisAddr,
		Username: "default",
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       1, // Use DB 1 for tests
	})
	s.Require().NoError(err, "Failed to create Redis client")

	// Verify connection
	err = rdb.Ping(s.Ctx).Err()
	s.Require().NoError(err, "Failed to ping Redis")

	s.Redis = rdb
}

func (s *TestSuite) setupServices() {
	var err error

	kafkaAddr := os.Getenv("KAFKA_ADDR")
	if kafkaAddr == "" {
		kafkaAddr = "localhost:9093"
	}

	// Chat service
	s.ChatSvc, err = chat.NewChatService(s.Ctx, s.Redis, s.Queries, kafkaAddr)
	s.Require().NoError(err, "Failed to create ChatService")
	s.Require().NotNil(s.ChatSvc, "ChatService should not be nil")

	// Friend service
	s.FriendSvc = friends.NewFriendService(s.Queries)
	s.Require().NotNil(s.FriendSvc, "FriendService should not be nil")

	// Session manager
	s.SessionMgr = sessions.NewSessionManager(s.Redis)
	s.Require().NotNil(s.SessionMgr, "SessionManager should not be nil")
}

func (s *TestSuite) cleanDatabase() {
	// Clean all tables in reverse dependency order
	_, err := s.DB.Exec("DELETE FROM friends")
	if err != nil {
		log.Printf("Warning: failed to clean friends table: %v", err)
	}

	_, err = s.DB.Exec("DELETE FROM users")
	if err != nil {
		log.Printf("Warning: failed to clean users table: %v", err)
	}
}

func (s *TestSuite) cleanRedis() {
	// Flush test database
	err := s.Redis.FlushDB(s.Ctx).Err()
	if err != nil {
		log.Printf("Warning: failed to flush Redis: %v", err)
	}
}

// Helper: Create test user
func (s *TestSuite) CreateTestUser(username, password string) db.User {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	s.Require().NoError(err, "Failed to hash password")

	user, err := s.Queries.CreateUser(s.Ctx, db.CreateUserParams{
		Username:     username,
		PasswordHash: string(hash),
		Icon:         sql.NullString{String: "gradient-blue", Valid: true},
		CustomIcon:   sql.NullString{},
	})
	s.Require().NoError(err, "Failed to create user")

	return user
}

// Helper: Create test session
func (s *TestSuite) CreateTestSession(username, userID string) string {
	sessionID := uuid.NewString()
	session := sessions.NewSession(
		sessionID,
		userID,
		username,
		time.Now().Unix(),
		time.Now().Unix(),
	)

	err := s.SessionMgr.SaveSession(s.Ctx, session)
	s.Require().NoError(err, "Failed to save session")

	return sessionID
}
