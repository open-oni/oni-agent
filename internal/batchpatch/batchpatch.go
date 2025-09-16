package batchpatch

import (
	"bufio"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
)

// Operation is a string that represents a single type of change in a batch
// patch, such as removing an issue
type Operation string

// OpRemoveIssue requests an issue be removed from the new batch
const OpRemoveIssue Operation = "RemoveIssue"

var validOps = []Operation{OpRemoveIssue}

// An instruction is a single operation to perform on a batch, such as removing
// a single issue
type instruction struct {
	operation Operation
	operand   string
}

// A BatchPatch is a series of instructions to apply to a batch as a single
// atomic change
type BatchPatch struct {
	batchName    string
	instructions []*instruction
}

// FromStream converts newline-delimited instructions into a structure suitable
// for correcting a batch
func FromStream(r io.Reader) (*BatchPatch, error) {
	var bp = &BatchPatch{}
	var scanner = bufio.NewScanner(r)

	var lineNum = 0
	for scanner.Scan() {
		lineNum++
		var line = strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if bp.batchName == "" {
			bp.batchName = strings.TrimSpace(scanner.Text())
			continue
		}

		var parts = strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("malformed instruction on line %d (%q)", lineNum, line)
		}

		var operation, operand = Operation(parts[0]), parts[1]
		if !slices.Contains(validOps, operation) {
			return nil, fmt.Errorf("invalid operation %q on line %d (%q)", operation, lineNum, line)
		}

		bp.instructions = append(bp.instructions, &instruction{operation: operation, operand: operand})
	}

	var err = scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("reading input stream: %w", err)
	}

	return bp, nil
}

// BatchName returns the name of the batch to which this BatchPatch will apply
func (bp *BatchPatch) BatchName() string {
	return bp.batchName
}

// Instructions yields one operation/operand pair per iteration. Operation is
// what a patch instruction does (e.g., "remove") while operand is what it
// affects (e.g., an issue key).
func (bp *BatchPatch) Instructions() iter.Seq2[Operation, string] {
	return func(yield func(operation Operation, operand string) bool) {
		for _, i := range bp.instructions {
			if !yield(i.operation, i.operand) {
				return
			}
		}
	}
}
