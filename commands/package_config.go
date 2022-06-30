package commands

import (
	"os"
	"reflect"
	"strconv"

	"github.com/Southclaws/sampctl/config"
	"github.com/Southclaws/sampctl/download"
	"github.com/Southclaws/sampctl/print"
	"github.com/jedib0t/go-pretty/v6/table"

	"github.com/pkg/errors"
	"gopkg.in/urfave/cli.v1"
)

var packageConfigFlags = []cli.Flag{
	//
}

func packageConfig(c *cli.Context) error {
	if c.Bool("verbose") {
		print.SetVerbose()
	}

	field := c.Args().Get(0) // the name of the config field
	value := c.Args().Get(1) // the value of the config value

	cnf := cfg
	v := reflect.ValueOf(cnf).Elem()

	// show output of fields
	if field == "" {
		displayConfig(v)
		return nil
	}

	if value == "" {
		return errors.Errorf("no value was set for field: %s", field)
	}

	f := v.FieldByName(field)
	if !f.IsValid() {
		return errors.New("invalid config field")
	}

	vt, _ := reflect.TypeOf(cnf).Elem().FieldByName(field)
	tag := vt.Tag.Get("json")
	if tag == "-" {
		return errors.New("invalid config field")
	}

	if !f.CanSet() {
		return errors.New("config field can not be written to")
	}

	switch f.Kind() {
	case reflect.String:
		{
			f.SetString(value)
		}
	case reflect.Pointer:
		{
			fieldType := reflect.Indirect(f)
			switch fieldType.Kind() {
			case reflect.Bool:
				{
					pValue, err := strconv.ParseBool(value)
					if err != nil {
						return errors.New("field requires a value which is type of bool")
					}
					f.Set(reflect.ValueOf(&pValue))
				}
			}
		}
	}

	config.WriteConfig(download.GetCacheDir(), *cnf)
	print.Info("successfully set config field", field, "to", value)

	return nil
}

func displayConfig(v reflect.Value) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Name", "Value"})

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := v.Type().Field(i)

		switch field.Kind() {
		case reflect.String:
			{
				v := field.String()
				t.AppendRows([]table.Row{
					{fieldType.Name, v},
				})
			}
		case reflect.Pointer:
			{
				pField := reflect.Indirect(field)
				switch pField.Kind() {
				case reflect.Bool:
					{
						t.AppendRows([]table.Row{
							{fieldType.Name, pField.Bool()},
						})
					}
				}
			}
		case reflect.Bool:
			{
				t.AppendRows([]table.Row{
					{fieldType.Name, field.Bool()},
				})
			}
		}
	}

	t.Render()
}
