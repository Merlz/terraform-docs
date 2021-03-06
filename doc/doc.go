package doc

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/hcl/ast"
)

const required = "required"

// Provider represents a terraform provider block.
type Provider struct {
	Name          string
	Documentation string
}

// Resource represents a terraform resource block.
type Resource struct {
	Name          string
	Type          string
	Documentation string
}

// Input represents a terraform input variable.
type Input struct {
	Name        string
	Description string
	Default     *Value
	Type        string
}

// Value returns the default value as a string.
func (i *Input) Value() string {
	if i.Default != nil {
		switch i.Default.Type {
		case "string":
			return i.Default.Literal
		case "map":
			return "<map>"
		case "list":
			return "<list>"
		}
	}

	return required
}

// Value represents a terraform value.
type Value struct {
	Type    string
	Literal string
}

// Output represents a terraform output.
type Output struct {
	Name        string
	Description string
}

// Doc represents a terraform module doc.
type Doc struct {
	Version   string
	Comment   string
	Providers []Provider
	Resources []Resource
	Inputs    []Input
	Outputs   []Output
}

type inputsByName []Input

func (a inputsByName) Len() int           { return len(a) }
func (a inputsByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a inputsByName) Less(i, j int) bool { return a[i].Name < a[j].Name }

type outputsByName []Output

func (a outputsByName) Len() int           { return len(a) }
func (a outputsByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a outputsByName) Less(i, j int) bool { return a[i].Name < a[j].Name }

type inputsByRequired []Input

func (a inputsByRequired) Len() int      { return len(a) }
func (a inputsByRequired) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a inputsByRequired) Less(i, j int) bool {
	switch {
	// i required, j not: i gets priority
	case a[i].Value() == required && a[j].Value() != required:
		return true
	// j required, i not: i does not get priority
	case a[i].Value() != required && a[j].Value() == required:
		return false
	// Otherwise, sort by name
	default:
		return a[i].Name < a[j].Name
	}
}

// Create creates a new *Doc from the supplied map
// of filenames and *ast.File.
func Create(files map[string]*ast.File, sortByRequired bool) *Doc {
	doc := new(Doc)

	for name, f := range files {
		list := f.Node.(*ast.ObjectList)

		required_version := version(list)
		if len(required_version) > 0 {
			doc.Version = required_version
		}

		doc.Providers = append(doc.Providers, providers(list)...)
		doc.Resources = append(doc.Resources, resources(list)...)
		doc.Inputs = append(doc.Inputs, inputs(list)...)
		doc.Outputs = append(doc.Outputs, outputs(list)...)

		filename := path.Base(name)
		comments := f.Comments

		if filename == "main.tf" && len(comments) > 0 {
			doc.Comment = header(comments[0])
		}
	}

    	switch {
    	case sortByRequired:
    		sort.Sort(inputsByRequired(doc.Inputs))
    	default:
    		sort.Sort(inputsByName(doc.Inputs))
    	}
	sort.Sort(inputsByName(doc.Inputs))
	sort.Sort(outputsByName(doc.Outputs))
	return doc
}

// Version returns the terraform version_required string from 'list'.
func version(list *ast.ObjectList) string {
	var ret string

	for _, item := range list.Items {
		if is(item, "terraform") && item.Val.(*ast.ObjectType).List.Items[0].Keys[0].Token.Text == "required_version" {
			version := item.Val.(*ast.ObjectType).List.Items[0].Val.(*ast.LiteralType).Token.Text
			version = strings.Trim(version, "\"")

			if len(version) > 0 {
				ret = version
			}
		}
	}

	return ret
}

// Providers returns all providers from 'list' along with links
// to their Terraform documentation.
func providers(list *ast.ObjectList) []Provider {
	var ret []Provider

	for _, item := range list.Items {
		if is(item, "provider") {
			name := item.Keys[1].Token.Text
			name = strings.Trim(name, "\"")
			link := fmt.Sprintf("https://www.terraform.io/docs/providers/%s", name)

			ret = append(ret, Provider{
				Name:          name,
				Documentation: link,
			})
		}
	}

	return ret
}

// Resources returns all resources from 'list' along with links
// to their Terraform documentation.
func resources(list *ast.ObjectList) []Resource {
	var ret []Resource

	for _, item := range list.Items {
		if is(item, "resource") {
			name := item.Keys[2].Token.Text
			name = strings.Trim(name, "\"")

			resourceType := item.Keys[1].Token.Text
			resourceType = strings.Trim(resourceType, "\"")

			resourceTypes := strings.SplitN(resourceType, "_", 2)
			namespace := resourceTypes[0]
			item := resourceTypes[1]
			link := fmt.Sprintf("https://www.terraform.io/docs/providers/%s/r/%s.html", namespace, item)

			ret = append(ret, Resource{
				Name:          name,
				Type:          resourceType,
				Documentation: link,
			})
		}
	}

	return ret
}

// Inputs returns all variables from `list`.
func inputs(list *ast.ObjectList) []Input {
	var ret []Input

	for _, item := range list.Items {
		if is(item, "variable") {
			name, _ := strconv.Unquote(item.Keys[1].Token.Text)
			if name == "" {
				name = item.Keys[1].Token.Text
			}
			items := item.Val.(*ast.ObjectType).List.Items
			var desc string
			switch {
			case description(items) != "":
				desc = description(items)
			case item.LeadComment != nil:
				desc = comment(item.LeadComment.List)
			}

			var itemsType = get(items, "type")
			var itemType string

			if itemsType == nil || itemsType.Literal == "" {
				itemType = "string"
			} else {
				itemType = itemsType.Literal
			}

			def := get(items, "default")
			ret = append(ret, Input{
				Name:        name,
				Description: desc,
				Default:     def,
				Type:        itemType,
			})
		}
	}

	return ret
}

// Outputs returns all outputs from `list`.
func outputs(list *ast.ObjectList) []Output {
	var ret []Output

	for _, item := range list.Items {
		if is(item, "output") {
			name, _ := strconv.Unquote(item.Keys[1].Token.Text)
			if name == "" {
				name = item.Keys[1].Token.Text
			}
			items := item.Val.(*ast.ObjectType).List.Items
			var desc string
			switch {
			case description(items) != "":
				desc = description(items)
			case item.LeadComment != nil:
				desc = comment(item.LeadComment.List)
			}

			ret = append(ret, Output{
				Name:        name,
				Description: strings.TrimSpace(desc),
			})
		}
	}

	return ret
}

// Get `key` from the list of object `items`.
func get(items []*ast.ObjectItem, key string) *Value {
	for _, item := range items {
		if is(item, key) {
			v := new(Value)

			if lit, ok := item.Val.(*ast.LiteralType); ok {
				if value, ok := lit.Token.Value().(string); ok {
					v.Literal = value
				} else {
					v.Literal = lit.Token.Text
				}
				v.Type = "string"
				return v
			}

			if _, ok := item.Val.(*ast.ObjectType); ok {
				v.Type = "map"
				return v
			}

			if _, ok := item.Val.(*ast.ListType); ok {
				v.Type = "list"
				return v
			}

			return nil
		}
	}

	return nil
}

// description returns a description from items or an empty string.
func description(items []*ast.ObjectItem) string {
	if v := get(items, "description"); v != nil {
		return v.Literal
	}

	return ""
}

// Is returns true if `item` is of `kind`.
func is(item *ast.ObjectItem, kind string) bool {
	if len(item.Keys) > 0 {
		return item.Keys[0].Token.Text == kind
	}

	return false
}

// Unquote the given string.
func unquote(s string) string {
	s, _ = strconv.Unquote(s)
	return s
}

// Comment cleans and returns a comment.
func comment(l []*ast.Comment) string {
	var line string
	var ret string

	for _, t := range l {
		line = strings.TrimSpace(t.Text)
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimPrefix(line, "//")
		ret += strings.TrimSpace(line) + "\n"
	}

	return ret
}

// Header returns the header comment from the list
// or an empty comment. The head comment must start
// at line 1 and start with `/**`.
func header(c *ast.CommentGroup) (comment string) {
	if len(c.List) == 0 {
		return comment
	}

	if c.Pos().Line != 1 {
		return comment
	}

	cm := strings.TrimSpace(c.List[0].Text)

	if strings.HasPrefix(cm, "/**") {
		lines := strings.Split(cm, "\n")

		if len(lines) < 2 {
			return comment
		}

		lines = lines[1 : len(lines)-1]
		for _, l := range lines {
			l = strings.TrimSpace(l)
			switch {
			case strings.TrimPrefix(l, "* ") != l:
				l = strings.TrimPrefix(l, "* ")
			default:
				l = strings.TrimPrefix(l, "*")
			}
			comment += l + "\n"
		}
	}

	return comment
}
