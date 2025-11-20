package command

import "flag"

type Executor struct {
	Name string
	Val  string
}

func (e *Executor) Execute() string {
	name := flag.String("s", e.Name, "Name of the service")
	value := flag.String("v", e.Val, "Value of the service")

	flag.Parse()

	return e.ExecuteBuilder(name, value)
}

func (e *Executor) ExecuteBuilder(name *string, value *string) string {
	var service Command
	switch *name {
	case "service-restart":
		service = &Restart{ServiceName: *value}
	case "service-status":
		service = &Status{ServiceName: *value}
	case "user-set-admin":
		service = &UserSetAdmin{Email: *value}
	default:
		return "Unknown command"
	}
	return service.Execute()
}
