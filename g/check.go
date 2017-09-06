package g

import (
	"fmt"
	"sync"

	"github.com/Masterminds/glide/msg"
)

var lock *sync.Mutex = &sync.Mutex{}

func Check(b bool, mess string, err error) {
	/*
		if b {
			if err != nil {
				//msg.Err(mess, err.Error())
				msg.Warn(" " + mess)
				msg.Err("Error Details : ")
				panic(err.Error())
			} else {
				//msg.Err(mess, err)
				msg.Warn(" " + mess)
				msg.Err("Error Details : ")
				panic(mess)
			}
		}
	*/
	if b {
		if err != nil {
			msg.Err(""+mess, err.Error())
		} else {
			msg.Err(""+mess, err)

		}
		Gret = false
		fmt.Println("")
	}
}

func GoBack() {
	lock.Lock()
	if Gret {
		msg.Warn("Gret status error, can not go back")
		return
	}
	Gret = true
	lock.Unlock()
}
