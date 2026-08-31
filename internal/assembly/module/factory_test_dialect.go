package moduleassembly

import (
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	ormdialect "github.com/domainry/domainry-orm/dialect"
)

func dialectForTest() (modulehost.Dialect, error) {
	dialect, err := ormdialect.New(ormdialect.SQLite)
	if err != nil {
		return nil, err
	}
	return dialect.WithSchema(""), nil
}
