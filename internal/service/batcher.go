package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/shauntso/hoodb/internal/raft"
)

// writeRequest 单个写入请求
type writeRequest struct {
	key   string
	value string
	errCh chan error
}

// writeBatcher 写入批处理器
// 将多个并发写入请求合并为一次 Raft 提交，大幅提升吞吐量
type writeBatcher struct {
	kv        *KVService
	reqCh     chan *writeRequest
	maxBatch  int           // 最大批量大小
	maxDelay  time.Duration // 最大等待时间
	stopCh    chan struct{}
	stoppedCh chan struct{}
	once      sync.Once
}

// newWriteBatcher 创建写入批处理器
func newWriteBatcher(kv *KVService, maxBatch int, maxDelay time.Duration) *writeBatcher {
	wb := &writeBatcher{
		kv:        kv,
		reqCh:     make(chan *writeRequest, maxBatch*4),
		maxBatch:  maxBatch,
		maxDelay:  maxDelay,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
	go wb.run()
	return wb
}

// Submit 提交一个写入请求，等待批处理完成
func (wb *writeBatcher) Submit(key, value string) error {
	req := &writeRequest{
		key:   key,
		value: value,
		errCh: make(chan error, 1),
	}

	select {
	case wb.reqCh <- req:
	case <-wb.stopCh:
		return fmt.Errorf("batcher stopped")
	}

	select {
	case err := <-req.errCh:
		return err
	case <-wb.stopCh:
		return fmt.Errorf("batcher stopped")
	}
}

// run 批处理主循环
func (wb *writeBatcher) run() {
	defer close(wb.stoppedCh)

	for {
		// 等待第一个请求
		var first *writeRequest
		select {
		case first = <-wb.reqCh:
		case <-wb.stopCh:
			return
		}

		// 收集更多请求 (在 maxDelay 窗口内)
		batch := []*writeRequest{first}
		timer := time.NewTimer(wb.maxDelay)

	collect:
		for len(batch) < wb.maxBatch {
			select {
			case req := <-wb.reqCh:
				batch = append(batch, req)
			case <-timer.C:
				break collect
			case <-wb.stopCh:
				timer.Stop()
				// 处理已收集的请求
				wb.executeBatch(batch)
				return
			}
		}
		timer.Stop()

		// 执行批量提交
		wb.executeBatch(batch)
	}
}

// executeBatch 执行一批写入
func (wb *writeBatcher) executeBatch(batch []*writeRequest) {
	if len(batch) == 0 {
		return
	}

	// 构建批量命令
	if len(batch) == 1 {
		// 单个请求直接提交
		cmd := &raft.Command{
			Op:    "set",
			Key:   batch[0].key,
			Value: batch[0].value,
		}
		err := wb.kv.raft.ApplyCommand(cmd)
		batch[0].errCh <- err
		return
	}

	// 多个请求合并为 batch_set
	kvs := make(map[string]string, len(batch))
	for _, req := range batch {
		kvs[req.key] = req.value
	}

	cmd := &raft.Command{
		Op:    "batch_set",
		Batch: kvs,
	}

	err := wb.kv.raft.ApplyCommand(cmd)

	// 通知所有请求
	for _, req := range batch {
		req.errCh <- err
	}
}

// Stop 停止批处理器
func (wb *writeBatcher) Stop() {
	wb.once.Do(func() {
		close(wb.stopCh)
		<-wb.stoppedCh
	})
}
