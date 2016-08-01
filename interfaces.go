package main

type Reloadable interface {
	Reload() error
}

type Stoppable interface {
	Stop() error
}
