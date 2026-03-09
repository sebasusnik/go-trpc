package higher_order

import "context"

type router struct{}

func (router) Query()    {}
func (router) Mutation() {}

var gotrpc router

type Router struct {
	procedures map[string]interface{}
}

func NewRouter() *Router                       { return &Router{} }
func (r *Router) Merge(prefix string, c *Router) {}

var r = NewRouter()
var taskRouter = NewRouter()

// Domain types
type Task struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
}

type CreateTaskInput struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type TaskStore struct{}

// Higher-order handlers (return functions)
func ListTasks(store *TaskStore) func(ctx context.Context, input struct{}) ([]Task, error) {
	return nil
}

func CreateTask(store *TaskStore) func(ctx context.Context, input CreateTaskInput) (Task, error) {
	return nil
}

// Direct function handler
func HealthCheck(ctx context.Context, input struct{}) (string, error) {
	return "ok", nil
}

func setup() {
	gotrpc.Query(taskRouter, "list", ListTasks(nil))
	gotrpc.Mutation(taskRouter, "create", CreateTask(nil))
	gotrpc.Query(r, "health", HealthCheck)
	r.Merge("task", taskRouter)
}
