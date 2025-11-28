package workflow

type Context struct {
	jobId  string
	extras map[any]any
}

func (c Context) GetJobId() string {
	return c.jobId
}

func (c Context) WithValue(k any, v any) Context {
	c.extras[k] = v
	return c
}

func (c Context) Value(k any) any {
	return c.extras[k]
}
