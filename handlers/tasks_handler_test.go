package handlers

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SysTechSalihY/mini-s3-clone/db"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGetTaskProgress(t *testing.T) {
	DB := setupTestDB(t)

	// Create users
	owner := db.User{
		ID:        "user-1",
		Email:     "owner@example.com",
		AccessKey: uuid.NewString(),
	}
	require.NoError(t, DB.Create(&owner).Error)

	otherUser := db.User{
		ID:        "user-2",
		Email:     "other@example.com",
		AccessKey: uuid.NewString(),
	}
	require.NoError(t, DB.Create(&otherUser).Error)

	// Create a task for the owner
	task := db.Task{
		ID:        "task-1",
		UserID:    owner.ID,
		Status:    "running",
		Progress:  50,
		Message:   "halfway done",
		CreatedAt: time.Now(),
	}
	require.NoError(t, DB.Create(&task).Error)

	tests := []struct {
		name       string
		user       *db.User
		taskID     string
		wantStatus int
		authSet    bool
	}{
		{
			name:       "valid request",
			user:       &owner,
			taskID:     "task-1",
			wantStatus: 200,
			authSet:    true,
		},
		{
			name:       "task not found",
			user:       &owner,
			taskID:     "nonexistent",
			wantStatus: 404,
			authSet:    true,
		},
		{
			name:       "forbidden access",
			user:       &otherUser,
			taskID:     "task-1",
			wantStatus: 403,
			authSet:    true,
		},
		{
			name:       "unauthenticated",
			user:       nil,
			taskID:     "task-1",
			wantStatus: 401,
			authSet:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/tasks/:taskID", func(c *fiber.Ctx) error {
				if tt.authSet {
					c.Locals("user", tt.user)
				}
				return GetTaskProgress(DB)(c)
			})

			req := httptest.NewRequest("GET", "/tasks/"+tt.taskID, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}
