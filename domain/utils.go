package domain

import "strings"

func (k Kind) String() string {
	return strings.Join([]string{k.Group, k.Version, k.Resource}, "/")
}
