package domain

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

const qemuDomainNamespace = "http://libvirt.org/schemas/domain/qemu/1.0"

type VMXMLIdentity struct {
	Name string
	UUID string
}

type vmXMLIdentityEnvelope struct {
	XMLName xml.Name
	Name    string `xml:"name"`
	UUID    string `xml:"uuid"`
}

func ParseVMXMLIdentity(text string) (VMXMLIdentity, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return VMXMLIdentity{}, fmt.Errorf("%w: domain XML is empty", ErrInvalidInput)
	}
	decoder := xml.NewDecoder(strings.NewReader(text))
	decoder.Strict = true
	var parsed vmXMLIdentityEnvelope
	if err := decoder.Decode(&parsed); err != nil {
		return VMXMLIdentity{}, fmt.Errorf("%w: invalid domain XML: %v", ErrInvalidInput, err)
	}
	if parsed.XMLName.Local != "domain" {
		return VMXMLIdentity{}, fmt.Errorf("%w: root element must be domain", ErrInvalidInput)
	}
	identity := VMXMLIdentity{Name: strings.TrimSpace(parsed.Name), UUID: strings.TrimSpace(parsed.UUID)}
	if identity.Name == "" || identity.UUID == "" {
		return VMXMLIdentity{}, fmt.Errorf("%w: domain name and uuid are required", ErrInvalidInput)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return VMXMLIdentity{}, fmt.Errorf("%w: multiple root elements are not allowed", ErrInvalidInput)
	}

	hookDecoder := xml.NewDecoder(strings.NewReader(text))
	hookDecoder.Strict = true
	for {
		token, err := hookDecoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return VMXMLIdentity{}, fmt.Errorf("%w: invalid domain XML: %v", ErrInvalidInput, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(start.Name.Space), qemuDomainNamespace) {
			return VMXMLIdentity{}, fmt.Errorf("%w: QEMU namespace overrides are not allowed", ErrInvalidInput)
		}
	}
	return identity, nil
}
