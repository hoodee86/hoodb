package service

import (
	"fmt"
	"sync"

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
// 使用流水线架构: 收集与执行重叠，消除等待间隙
type writeBatcher struct {
	kv        *KVService
	reqCh     chan *writeRequest
	maxBatch  int // 最大批量大小
	stopCh    chan struct{}
	stoppedCh chan struct{}
	once      sync.Once
}

const numExecutors = 4 // 并行执行器数量, 允许多个 Raft Apply 同时进行

// newWriteBatcher 创建写入批处理器
func newWriteBatcher(kv *KVService, maxBatch int) *writeBatcher {
	wb := &writeBatcher{
		kv:        kv,
		reqCh:     make(chan *writeRequest, maxBatch*8),
		maxBatch:  maxBatch,
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

// run 批处理主循环 — 流水线架构
// 1. 收集阶段: 阻塞等待第一个请求, 然后立即 drain channel 中所有待处理请求
// 2. 发送到执行器 channel (不等待执行完成, 立即开始收集下一批)
// 3. 多个执行器协程并行调用 Raft Apply, 配合 BatchApplyCh 合并 BoltDB 写入
func (wb *writeBatcher) run() {
	defer close(wb.stoppedCh)

	// 执行器 channel: 收集完成的 batch 发送到这里, 执行器取出执行
	batchCh := make(chan []*writeRequest, numExecutors*2)

	// 启动多个并行执行器
	var execWg sync.WaitGroup
	for i := 0; i < numExecutors; i++ {
		execWg.Add(1)
		go func() {
			defer execWg.Done()
			for batch := range batchCh {
				wb.executeBatch(batch)
			}
		}()
	}

	defer func() {
		close(batchCh)
		execWg.Wait()
	}()

	for {
		// 阻塞等待第一个请求到达
		var first *writeRequest
		select {
		case first = <-wb.reqCh:
		case <-wb.stopCh:
			return
		}

		// 立即 drain: 非阻塞地取出 channel 中所有等待的请求
		// 无需 timer 等待 — 在高并发下 channel 中已有大量请求
		batch := []*writeRequest{first}
	drain:
		for len(batch) < wb.maxBatch {
			select {
			case req := <-wb.reqCh:
				batch = append(batch, req)
			default:
				break drain
			}
		}

		// 发送到执行器 (流水线: 立即回到收集下一批, 不等执行完)
		select {
		case batchCh <- batch:
		case <-wb.stopCh:
			wb.executeBatch(batch)
			return
		}
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
