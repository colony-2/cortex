package workflow

type Context struct {
	jobId string
}

func (c *Context) GetJobId() string {
	return c.jobId
}
