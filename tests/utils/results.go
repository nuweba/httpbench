package tests

import (
	"errors"
	"fmt"
	"github.com/nuweba/httpbench/syncedtrace"
	"reflect"
	"sync"
	"time"
)

// Test configurations generation
func generatePermutations(wg *sync.WaitGroup, channel chan []int64, result []int64, params ...[]int64) {
	defer wg.Done()
	if len(params) == 0 {
		channel <- result
		return
	}
	p, params := params[0], params[1:]
	for i := 0; i < len(p); i++ {
		wg.Add(1)
		// call self with remaining params
		go generatePermutations(wg, channel, append(result, p[i]), params...)
	}
	if len(p) == 0 { // even if input is empty, proceed for next index
		wg.Add(1)
		go generatePermutations(wg, channel, result, params...)
	}
}

func createPermutationChannel(fields [][]int64) chan []int64 {
	permutations := make(chan []int64)
	var wg sync.WaitGroup
	wg.Add(1)
	generatePermutations(&wg, permutations, []int64{}, fields...)
	go func() { wg.Wait(); close(permutations) }()
	return permutations
}

func GenerateBenchConfigs(inputBenches []BenchConfig, inputValues [][]int64) []BenchConfig {
	// Values should be at the same order they are set in `SetPermutation`
	permutations := createPermutationChannel(inputValues)

	var benchConfigs []BenchConfig
	for permutation := range permutations {
		for _, inputBench := range inputBenches {
			var bench BenchConfig
			bench = inputBench
			bench.SetPermutation(permutation)
			benchConfigs = append(benchConfigs, bench)
		}
	}
	return benchConfigs
}

func IsValueInRange(baseVal, refVal interface{}) error {
	//percentageRange := 5.0 * mul
	offset := (10 * time.Millisecond).Nanoseconds()
	if reflect.TypeOf(baseVal) != reflect.TypeOf(refVal) {
		return errors.New(fmt.Sprint("Value around: values have different types:", reflect.TypeOf(refVal), reflect.TypeOf(baseVal)))
	}

	var baseValInt, refValInt int64
	switch baseVal.(type) {
	case time.Time:
		baseValInt = int64(baseVal.(time.Time).UnixNano())
		refValInt = int64(refVal.(time.Time).UnixNano())
	case time.Duration:
		baseValInt = int64(baseVal.(time.Duration).Nanoseconds())
		refValInt = int64(refVal.(time.Duration).Nanoseconds())
	case int:
		baseValInt = int64(baseVal.(int))
		refValInt = int64(refVal.(int))
	default:
		return errors.New(fmt.Sprint("Types didn't matched", baseVal, refVal))
	}

	if baseValInt <= refValInt+offset && baseValInt >= refValInt-offset {
		return nil
	}
	return errors.New(fmt.Sprint("Values not in range:", baseVal, refVal))
}

func min(nums ...int64) int64 {
	number := nums[0]
	if len(nums) == 0 {
		return 0
	}
	for i := 1; i < len(nums); i++ {
		if nums[i] < number {
			number = nums[i]
		}
	}
	return number
}

func max(nums ...int64) int64 {
	if len(nums) == 0 {
		return 0
	}
	number := nums[0]
	for i := 1; i < len(nums); i++ {
		if nums[i] > number {
			number = nums[i]
		}
	}
	return number
}

func contains(s []syncedtrace.TraceHookType, e syncedtrace.TraceHookType) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
