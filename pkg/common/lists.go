package common

import (
	"cmp"
	"container/heap"
)

type (
	// Min-Heap
	Item[T cmp.Ordered] struct {
		value T
		queue int
	}
)

type MinHeap[T cmp.Ordered] []Item[T]

func (h MinHeap[T]) Len() int           { return len(h) }
func (h MinHeap[T]) Less(i, j int) bool { return h[i].value < h[j].value }
func (h MinHeap[T]) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *MinHeap[T]) Push(x any) {
	*h = append(*h, x.(Item[T]))
}

func (h *MinHeap[T]) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func Merge[T cmp.Ordered](queues [][]T) []T {
	total := 0
	h := &MinHeap[T]{}

	// Erstes Element jeder Schlange in den Heap
	for i, q := range queues {
		if len(q) > 0 {
			heap.Push(h, Item[T]{
				value: q[0],
				queue: i,
			})
			total += len(q)
		}
	}

	result := make([]T, 0, total)
	// Position des nächsten Elements jeder Schlange
	pos := make([]int, len(queues))

	for h.Len() > 0 {
		item := heap.Pop(h).(Item[T])
		if len(result) == 0 || item.value != result[len(result)-1] { // no duplicates
			result = append(result, item.value)
		}
		pos[item.queue]++
		if pos[item.queue] < len(queues[item.queue]) {
			heap.Push(h, Item[T]{
				value: queues[item.queue][pos[item.queue]],
				queue: item.queue,
			})
		}
	}
	return result
}
