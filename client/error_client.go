package client

// errorClient 是一个始终返回固定错误信息的 Client 实现。
// 用于 client 未登记时把"目标服务未配置"这个事实显式地传递给调用方,
// 避免历史行为里的两种问题:
//  1. 兜底拿到当前进程的 client,把请求打到错误的服务上;
//  2. 返回 nil,调用方后续 c.cc.Next() 触发 nil panic。
type errorClient struct {
	err error
}

// NewErrorClient 创建一个始终返回 err 的 Client
func NewErrorClient(err error) Client {
	if err == nil {
		err = ErrClientNotRegistered
	}
	return &errorClient{err: err}
}

// Init 直接返回错误
func (c *errorClient) Init(...Option) error { return c.err }

// Options 返回空的配置
func (c *errorClient) Options() Options { return Options{} }

// String 返回错误描述
func (c *errorClient) String() string { return "error-client: " + c.err.Error() }

// Next 直接返回错误,调用方会在 invoke 之前失败并拿到可读的错误信息
func (c *errorClient) Next() (*poolConn, error) { return nil, c.err }
