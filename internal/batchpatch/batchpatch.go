package batchpatch

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strings"
)

const opRemove = "remove"

var validOps = []string{opRemove}

// An instruction is a single operation to perform on a batch, such as removing
// a single issue
type instruction struct {
	operation string
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

		var operation, operand = parts[0], parts[1]
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
