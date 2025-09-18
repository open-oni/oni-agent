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

// BP is currently just a series of instructions to apply to a batch as a
// single atomic change
type BP struct {
	instructions []*instruction
}

func (bp *BP) String() string {
	if bp == nil {
		return ""
	}
	var lines []string
	for operation, operand := range bp.Instructions() {
		lines = append(lines, fmt.Sprintf("<%s>: %q", operation, operand))
	}

	return strings.Join(lines, "\n")
}

// FromStream converts newline-delimited instructions into a structure suitable
// for correcting a batch
func FromStream(r io.Reader) (*BP, error) {
	var bp = &BP{}
	var scanner = bufio.NewScanner(r)

	var lineNum = 0
	for scanner.Scan() {
		lineNum++
		var line = strings.TrimSpace(scanner.Text())
		if line == "" {
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

// Instructions yields one operation/operand pair per iteration. Operation is
// what a patch instruction does (e.g., "remove") while operand is what it
// affects (e.g., an issue key).
func (bp *BP) Instructions() iter.Seq2[Operation, string] {
	return func(yield func(operation Operation, operand string) bool) {
		for _, i := range bp.instructions {
			if !yield(i.operation, i.operand) {
				return
			}
		}
	}
}
