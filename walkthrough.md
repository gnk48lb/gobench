# GoBench Phase 2 (Step 2.1) Completed: Task Queue and Flow

I have successfully added the Redis queue, `TaskLog` tracking, and a simple background Worker to simulate task execution flow. I also resolved the blocking queue issue to ensure graceful server shutdowns.

## Key Changes
1. **Bug Fix**: Fixed a panic risk in `internal/handler/task.go` using safe type assertions for `userID`.
2. **Redis Setup**: Added `redis` configuration and a global client initialization using `go-redis`.
3. **Queue & TaskLog**:
   - `model.TaskLog` is now tracked and auto-migrated in the database.
   - Designed the `TaskMessage` in `queue` to pass both `TaskID` and `LogID`. This bridges the Trigger endpoint with the background worker seamlessly.
   - Handled non-blocking Redis timeout on `BRPOP` (2s timeout), so the Worker correctly catches the shutdown signal.
4. **Worker Flow**: 
   - Start: A background goroutine reads from the Redis queue.
   - Step: Changes status to `running`, sleeps 1 second to simulate execution, and marks the task `success`.
   - Shutdown: Properly captures `w.stopCh` to cleanly exit during server `Graceful Shutdown`.

## Manual Testing (Using cURL)

You can trigger a task and monitor its logs using these commands:

> [!WARNING]
> You must run `go get github.com/redis/go-redis/v9` in your terminal to fetch the new dependencies before building! Also make sure your local Redis server is running on `localhost:6379`.

**1. Trigger a Task (Authenticated)**
```bash
# This creates a 'pending' TaskLog and LPUSHes a TaskMessage into Redis
curl -X POST http://localhost:8080/api/v1/tasks/1/trigger \
     -H "Authorization: Bearer <YOUR_TOKEN_HERE>"
```

**2. View Task Execution Logs (Authenticated)**
```bash
# Verify the status transitions from 'pending' -> 'running' -> 'success'
curl -X GET "http://localhost:8080/api/v1/tasks/1/logs" \
     -H "Authorization: Bearer <YOUR_TOKEN_HERE>"
```
