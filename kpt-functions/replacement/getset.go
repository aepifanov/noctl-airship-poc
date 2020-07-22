package replacement

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"log"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func parseFieldRef(in string) ([]string, error) {
	var cur bytes.Buffer
	out := []string{}
	var state int
	for i := 0; i < len(in); {
		r, size := utf8.DecodeRuneInString(in[i:])

		switch state {
		case 0: // initial state
			if r == '.' {
				if cur.String() != "" {
					out = append(out, cur.String())
					cur = bytes.Buffer{}
				}
			} else if r == '[' {
				if cur.String() != "" {
					out = append(out, cur.String())
					cur = bytes.Buffer{}
				}
				cur.WriteRune(r)
				state = 1
			} else {
				cur.WriteRune(r)
			}
		case 1: // state inside []
			cur.WriteRune(r)
			if r == ']' {
				state = 0
			}
		}
		i += size
	}

	if state != 0 {
		return nil, fmt.Errorf("unclosed [")
	}

	return append(out, cur.String()), nil
}

func seqNodeIndexPath(p string) (int64, error) {
	if p[0] == '[' && p[len(p)-1] == ']' {
		p = p[1 : len(p)-1]
	}
	i, err := strconv.ParseInt(p, 10, 64)
	if err != nil {
		return 0, err
	}
	if i < 0 {
		return 0, fmt.Errorf("index can't be negative. got %d", i)
	}
	return i, nil
}

func getFieldValue(node *yaml.RNode, fieldRef string) (interface{}, error) {
	node, err := getFieldValueImpl(node, strings.Split(fieldRef, "|"))
	if err != nil {
		return nil, err
	}
	if node.YNode().Kind == yaml.ScalarNode {
		return yaml.GetValue(node), nil
	}

	return node, nil
}

func getFieldValueImpl(node *yaml.RNode, fieldRefs []string) (*yaml.RNode, error) {
	path, err := parseFieldRef(fieldRefs[0])
	if err != nil {
		return nil, err
	}

	cn := node
	for _, p := range path {

		// index case
		if cn.YNode().Kind == yaml.SequenceNode {
			i, err := seqNodeIndexPath(p)
			if err == nil {
				content := cn.Content()
				if i >= int64(len(content)) {
					return nil, fmt.Errorf("index %d is too big", i)
				}
				cn = yaml.NewRNode(content[i])
				continue
			}
		}

		// default case - use loojup
		cn, err = cn.Pipe(yaml.Lookup(p))
		if err != nil {
			return nil, err
		}
	}

	if len(fieldRefs) == 1 {
		return cn, nil
	}

	if cn.YNode().Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("node %v isn't scalar", path)
	}

	node, err = yaml.Parse(yaml.GetValue(cn))
	if err != nil {
		return nil, err
	}

	return getFieldValueImpl(node, fieldRefs[1:])
}

func setFieldValue(node *yaml.RNode, fieldRef string, value interface{}) error {
	return setFieldValueImpl(node, strings.Split(fieldRef, "|"), value)
}

func setFieldValueImpl(node *yaml.RNode, fieldRefPart []string, value interface{}) error {
	//ds, _ := node.String()
	//log.Printf("setFieldValueImpl %s %v %s", ds, fieldRefPart, value)
	//defer log.Printf("exit setFieldValueImpl %s %v %s", ds, fieldRefPart, value)
	if len(fieldRefPart) > 1 {
		// this can be done only for string field
		//v, err := node.Pipe(yaml.Lookup(strings.Split(fieldRefPart[0], ".")...))
		v, err := node.Pipe(yaml.PathGetter{Path: strings.Split(fieldRefPart[0], ".")})
		if err != nil {
			return fmt.Errorf("wasn't able to lookup %s: %w", fieldRefPart[0], err)
		}
		if v == nil {
			return fmt.Errorf("wasn't able to find value for fieldref %s", fieldRefPart[0])
		}
		//log.Printf("parsing %s", yaml.GetValue(v))
		includedNode, err := yaml.Parse(yaml.GetValue(v))
		if err != nil {
			return fmt.Errorf("wasn't able to parse yaml value for fieldref %s", fieldRefPart[0])
		}
		err = setFieldValueImpl(includedNode, fieldRefPart[1:], value)
		if err != nil {
			return fmt.Errorf("recursive %s: %w", fieldRefPart[0], err)
		}
		s, err := includedNode.String()
		if err != nil {
			return fmt.Errorf("can't marshal includedNode: %w", err)
		}
		//log.Printf("setting %s", s)
		err = v.PipeE(yaml.FieldSetter{StringValue: s})
		if err != nil {
			return fmt.Errorf("can't set new value %s back: %w", s, err)
		}
		return nil

	}

	svalue, ok := value.(string)
	if ok {
		log.Printf("looking for %s", fieldRefPart[0])
		v, err := node.Pipe(yaml.LookupCreate(yaml.ScalarNode, strings.Split(fieldRefPart[0], ".")...))
		if err != nil {
			return fmt.Errorf("scalar case: wasn't able to lookup %v: %w", strings.Split(fieldRefPart[0], "."), err)
		}
		log.Printf("found %s", yaml.GetValue(v))
		err = v.PipeE(yaml.FieldSetter{StringValue: svalue})
		if err != nil {
			return fmt.Errorf("scalar case: fieldsetter returned error for %s: %w", fieldRefPart[0], err)
		}
		return nil
	}
	rnode, ok := value.(*yaml.RNode)
	if ok {
		path := strings.Split(fieldRefPart[0], ".")
		v := node
		if len(path) > 1 {
			var err error
			v, err = node.Pipe(yaml.Lookup(path[:len(path)-1]...))
			if err != nil {
				return fmt.Errorf("wasn't able to lookup %s: %w", fieldRefPart[0], err)
			}
		}
		log.Printf("setting Name %v to %v", path[len(path)-1], path[:len(path)-1])
		err := v.PipeE(yaml.FieldSetter{Name: path[len(path)-1], Value: rnode})
		if err != nil {
			return fmt.Errorf("fieldsetter returned error for %s: %w", fieldRefPart[0], err)
		}
		return nil
	}
	return fmt.Errorf("unexpected value type %v: %T", value, value)
}
