package basecontext

import "fmt"

type IBaseContext interface {
	PrintName()
	GetName() string
}

type ISContext interface {
	IBaseContext
	PrintInfo()
	GetInfo() string
}

type TBaseContext struct {
	name string
}

type TSContext struct {
	TBaseContext
	info string
}

func (this *TBaseContext) SetName(n string) {
	this.name = n
}

func (this *TBaseContext) getname() string {
	name := "__TBaseContext__"
	return this.name + name
}

func (this *TBaseContext) GetName() string {
	return this.getname()
}

func (this *TBaseContext) PrintName() {
	fmt.Println(this.GetName())
}

func (this *TSContext) SetInfo(n string) {
	this.info = n
}

func (this *TSContext) getinfo() string {
	info := "TSContext"
	return this.info + info
}

func (this *TSContext) GetInfo() string {
	return this.getinfo()
}

func (this *TSContext) PrintInfo() {
	fmt.Println(this.GetInfo())
}
