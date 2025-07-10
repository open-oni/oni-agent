package batchfix

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

// BatchXML holds the deserialized data from an ONI batch.xml file as well as
// correction-specific features like which directories will be skipped (based
// on passed in issue keys)
type BatchXML struct {
	XMLName   string      `xml:"http://www.loc.gov/ndnp batch"`
	Name      string      `xml:"name,attr"`
	Awardee   string      `xml:"awardee,attr"`
	AwardYear string      `xml:"awardYear,attr"`
	Issues    []*IssueXML `xml:"issue"`
	Reels     []*reelXML  `xml:"reel"`
	SkipDirs  []string    `xml:"-"`
}

// IssueXML holds the deserialized data from an ONI batch.xml for the
// individual issues as well as a flag for whether or not an issue is being
// removed in the corrected output.
type IssueXML struct {
	LCCN      string `xml:"lccn,attr"`
	IssueDate string `xml:"issueDate,attr"`
	Edition   string `xml:"editionOrder,attr"`
	Path      string `xml:",innerxml"`
	Skip      bool   `xml:"-"`
}

// String gives us a value for testing equality. We ignore edition for
// simplicity here, so this isn't the same as an NCA issue key.
func (i *IssueXML) String() string {
	return i.LCCN+"/"+i.IssueDate
}

type reelXML struct {
	ReelNum string `xml:"reelNumber,attr"`
	Path    string `xml:",innerxml"`
}

// ParseBatch reads the given XML file and processes it into batch data.  The
// skipKeys are converted into directories that should be skipped from the
// copy, and stored in the returned structure's SkipDirs field.
func ParseBatch(fs afero.Fs, pth string, skipKeys []string) (*BatchXML, error) {
	var data, err = afero.ReadFile(fs, pth)
	if err != nil {
		return nil, err
	}
	var b = new(BatchXML)
	err = xml.Unmarshal(data, b)
	if err != nil {
		return nil, err
	}
	if len(b.Issues) == 0 {
		return nil, fmt.Errorf("parsed data has no issues")
	}

	var keyToDir = make(map[string]*IssueXML)
	for _, issue := range b.Issues {
		var key = keyfix(issue.LCCN + "/" + issue.IssueDate + issue.Edition)
		keyToDir[key] = issue
	}

	for _, key := range skipKeys {
		key = keyfix(key)
		var issue = keyToDir[key]
		if issue == nil {
			return nil, fmt.Errorf("issuekey %q not in batch", key)
		}

		var dir, _ = filepath.Split(issue.Path)
		issue.Skip = true
		b.SkipDirs = append(b.SkipDirs, dir)
	}

	var newIssues []*IssueXML
	for _, i := range b.Issues {
		if i.Skip {
			continue
		}

		newIssues = append(newIssues, i)
	}

	b.Issues = newIssues
	return b, nil
}

func (b *BatchXML) WriteBatchXML(fs afero.Fs, pth string) error {
	var dir, _ = filepath.Split(pth)
	var err = fs.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	var data []byte
	data, err = xml.MarshalIndent(b, "", "\t")
	if err != nil {
		return err
	}

	var output = append([]byte(xml.Header), data...)

	return afero.WriteFile(fs, pth, output, 0644)
}

func keyfix(key string) string {
	key = strings.Replace(key, "-", "", -1)
	key = strings.Replace(key, "_", "", -1)
	return key
}

func makeattr(name, val string) xml.Attr {
	return xml.Attr{Name: xml.Name{Local: name}, Value: val}
}

// MarshalXML sets up a wrapper element that defines the <ndnp:batch> tag very
// precisely.
//
// This stupid hack seems to be necessary to get Go's XML encoding to output
// the namespaces we want so the batch XML opening tag looks basically the same
// as it did prior to the rewrite
func (b *BatchXML) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	var wrapper = struct {
		Issues []*IssueXML `xml:"issue"`
		Reels  []*reelXML  `xml:"reel"`
	}{
		Issues: b.Issues,
		Reels:  b.Reels,
	}

	start.Name.Local = "ndnp:batch"
	start.Attr = append(start.Attr, makeattr("xmlns:ndnp", "http://www.loc.gov/ndnp"))
	start.Attr = append(start.Attr, makeattr("xmlns:xsi", "http://www.w3.org/2001/XMLSchema-instance"))
	start.Attr = append(start.Attr, makeattr("xmlns", "http://www.loc.gov/ndnp"))
	start.Attr = append(start.Attr, makeattr("name", b.Name))
	start.Attr = append(start.Attr, makeattr("awardee", b.Awardee))
	start.Attr = append(start.Attr, makeattr("awardYear", b.AwardYear))

	return e.EncodeElement(wrapper, start)
}
