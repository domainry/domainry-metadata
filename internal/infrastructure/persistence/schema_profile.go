package persistence

import (
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	"github.com/domainry/domainry-metadata/internal/infrastructure/persistence/mysql"
	"github.com/domainry/domainry-orm/dialect"
)

var localizedTableAdapters = map[dialect.Name]func(modulehost.Dialect, string) string{
	dialect.MySQL: mysql.LocalizedTable,
}

func LocalizedTable(driver string, renderer modulehost.Dialect, statement string) (string, error) {
	parsed, err := dialect.Parse(driver)
	if err != nil {
		return "", err
	}
	if adapt := localizedTableAdapters[parsed.Name()]; adapt != nil {
		return adapt(renderer, statement), nil
	}
	return statement, nil
}
