package hype

// NewCodeNodes implements the ParseElementFn type
func NewCodeNodes(p *Parser, el *Element) (Nodes, error) {
	if el == nil {
		return nil, ErrIsNil("element")
	}

	ats := el.Attrs()

	if ats.Len() == 0 {
		return NewInlineCodeNodes(p, el)
	}

	if _, ok := ats.Get("src"); ok {
		return NewSourceCodeNodes(p, el)
	}

	return NewFencedCodeNodes(p, el)
}
