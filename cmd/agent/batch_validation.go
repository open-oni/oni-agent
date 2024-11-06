package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
)

// issue describes a single "issue" element in a batch XML file
type issue struct {
	LCCN         string `xml:"lccn,attr"`
	IssueDate    string `xml:"issueDate,attr"`
	EditionOrder string `xml:"editionOrder,attr"`
	Filepath     string `xml:",innerxml"`
}

// batch describes the data we care about which lives in a batch.xml file
type batch struct {
	Name   string   `xml:"name,attr"`
	Issues []*issue `xml:"issue"`
}

// validateBatch checks that the path exists, that there's a manifest file, and
// that the paths to the issues' files exist. We don't try to do further
// validations to ensure things like the JP2s are valid or anything as this
// needs to be a fairly quick check.
//
// Note that we only check for the batch.xml, not batch_1.xml: NCA doesn't do
// the DVV stuff chronam batches had, and validates XML doesn't give us
// anything that isn't in the main file anyway.
func validateBatch(batchPath string) error {
	var xmlfile = filepath.Join(batchPath, "batch.xml")
	var data, err = os.ReadFile(xmlfile)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	var b batch
	err = xml.Unmarshal(data, &b)
	if err != nil {
		return fmt.Errorf("processing xml: %w", err)
	}

	for _, i := range b.Issues {
		var fp = i.Filepath
		if fp[0] == '.' {
			fp = filepath.Join(batchPath, fp)
		}
		var info, err = os.Stat(fp)
		if err != nil {
			return fmt.Errorf("checking issue file %s: %w", fp, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("checking issue file %s: not a regular file", fp)
		}
	}

	return nil
}
