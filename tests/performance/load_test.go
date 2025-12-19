package performance_test

import (
	"context"
	"exc6/db"
	"exc6/services/sessions"
	"exc6/tests/setup"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestConcurrentMessageSending(t *testing.T) {
	suite := &setup.TestSuite{}
	suite.SetupSuite()
	defer suite.TearDownSuite()

	user1 := suite.CreateTestUser("load1", "pass123")
	user2 := suite.CreateTestUser("load2", "pass123")

	// Send 100 messages concurrently
	var wg sync.WaitGroup
	errors := make(chan error, 100)

	start := time.Now()

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := suite.ChatSvc.SendMessage(
				ctx,
				user1.Username,
				user2.Username,
				fmt.Sprintf("Message %d", num),
			)

			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	duration := time.Since(start)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Error: %v", err)
		errorCount++
	}

	assert.Equal(t, 0, errorCount, "Should have no errors")
	assert.Less(t, duration, 10*time.Second, "Should complete within 10 seconds")

	t.Logf("Sent 100 messages in %v", duration)
	t.Logf("Average: %v per message", duration/100)
}

func TestConcurrentUserRegistration(t *testing.T) {
	suite := &setup.TestSuite{}
	suite.SetupSuite()
	defer suite.TearDownSuite()

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Try to register 50 users concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()

			username := fmt.Sprintf("concurrent%d", num)
			hash, _ := bcrypt.GenerateFromPassword([]byte("pass123"), bcrypt.DefaultCost)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := suite.Queries.CreateUser(ctx, db.CreateUserParams{
				Username:     username,
				PasswordHash: string(hash),
			})

			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, 50, successCount, "All registrations should succeed")
}

func TestSessionCreationThroughput(t *testing.T) {
	suite := &setup.TestSuite{}
	suite.SetupSuite()
	defer suite.TearDownSuite()

	user := suite.CreateTestUser("throughput", "pass123")

	start := time.Now()
	sessionCount := 1000

	for i := 0; i < sessionCount; i++ {
		sessionID := uuid.NewString()
		session := sessions.NewSession(
			sessionID,
			user.ID.String(),
			user.Username,
			time.Now().Unix(),
			time.Now().Unix(),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		err := suite.SessionMgr.SaveSession(ctx, session)
		cancel()

		assert.NoError(t, err)
	}

	duration := time.Since(start)
	throughput := float64(sessionCount) / duration.Seconds()

	t.Logf("Created %d sessions in %v", sessionCount, duration)
	t.Logf("Throughput: %.2f sessions/second", throughput)

	assert.Greater(t, throughput, 100.0, "Should create at least 100 sessions/second")
}

func BenchmarkMessageSending(b *testing.B) {
	suite := &setup.TestSuite{}
	suite.SetupSuite()
	defer suite.TearDownSuite()

	user1 := suite.CreateTestUser("bench1", "pass123")
	user2 := suite.CreateTestUser("bench2", "pass123")

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		suite.ChatSvc.SendMessage(ctx, user1.Username, user2.Username, "Benchmark message")
	}
}

func BenchmarkSessionRetrieval(b *testing.B) {
	suite := &setup.TestSuite{}
	suite.SetupSuite()
	defer suite.TearDownSuite()

	user := suite.CreateTestUser("benchsession", "pass123")
	sessionID := suite.CreateTestSession(user.Username, user.ID.String())

	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		suite.SessionMgr.GetSession(ctx, sessionID)
	}
}
