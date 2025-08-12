package main

import (
	"fmt"
	"time"

	Broom "github.com/Amirhos-esm/Broom"
)

func main() {

	broom := Broom.NewBroom(time.Second * 5)
	broom.Run()

	folder := "E:/projects/broom/list/f1" 

	fmt.Println("add folder: ",broom.AddFolder(folder,Broom.KByte * 10))
	fmt.Println("add folder: ",broom.AddFolder(folder,Broom.KByte * 10))

	time.Sleep(6)
	f , err := broom.GetFolder(folder)
	fmt.Println("get folder: ", err, f)

	fmt.Println("remove : ",broom.RemoveFolder(folder))

	fmt.Println("remove : ",broom.RemoveFolder(folder))

	time.Sleep(300 * time.Second)
	broom.Stop()
}
