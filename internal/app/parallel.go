package app

import "sync"

func parallelMap[Input, Output any](
	items []Input,
	limit int,
	transform func(Input) Output,
) []Output {
	results := make([]Output, len(items))
	jobs := make(chan int)
	workers := min(limit, len(items))

	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			for index := range jobs {
				results[index] = transform(items[index])
			}
		})
	}
	for index := range items {
		jobs <- index
	}
	close(jobs)
	group.Wait()

	return results
}
