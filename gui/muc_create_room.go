package gui

import (
	"errors"
	"sync"

	"github.com/coyim/coyim/coylog"

	"github.com/coyim/coyim/xmpp/jid"
	"github.com/coyim/gotk3adapter/gtki"
)

type mucCreateRoomView struct {
	u *gtkUI

	autoJoin bool
	cancel   chan bool

	dialog    gtki.Dialog `gtk-widget:"create-room-dialog"`
	container gtki.Box    `gtk-widget:"create-room-content"`

	form    *mucCreateRoomViewForm
	success *mucCreateRoomViewSuccess

	showCreateForm  func()
	showSuccessView func(*account, jid.Bare)

	onAutoJoin *callbacksSet
	onDestroy  *callbacksSet

	sync.Mutex
}

func newCreateMUCRoomView(u *gtkUI) *mucCreateRoomView {
	v := &mucCreateRoomView{
		u:               u,
		showCreateForm:  func() {},
		showSuccessView: func(*account, jid.Bare) {},
		onAutoJoin:      newCallbacksSet(),
		onDestroy:       newCallbacksSet(),
	}

	v.initBuilder()
	v.initChildViews()

	return v
}

func (v *mucCreateRoomView) initBuilder() {
	builder := newBuilder("MUCCreateRoomDialog")
	panicOnDevError(builder.bindObjects(v))

	builder.ConnectSignals(map[string]interface{}{
		"on_close_window": v.onCloseWindow,
	})
}

func (v *mucCreateRoomView) initChildViews() {
	v.form = v.initCreateRoomForm()
	v.showCreateForm = func() {
		v.form.showCreateForm(v)
	}

	v.success = v.initCreateRoomSuccess()
	v.showSuccessView = func(ca *account, roomID jid.Bare) {
		v.success.showSuccessView(v, ca, roomID)
	}
}

func (v *mucCreateRoomView) onCancel() {
	if v.cancel != nil {
		v.cancel <- true
		v.cancel = nil
	}

	v.dialog.Destroy()
}

func (v *mucCreateRoomView) onCloseWindow() {
	v.onDestroy.invokeAll()
}

var (
	errCreateRoomCheckIfExistsFails = errors.New("room exists failed")
	errCreateRoomAlreadyExists      = errors.New("room already exists")
	errCreateRoomFailed             = errors.New("couldn't create the room")
)

func (v *mucCreateRoomView) checkIfRoomExists(ca *account, roomID jid.Bare, result chan bool, errors chan error) {
	rc, ec := ca.session.HasRoom(roomID, nil)
	go func() {
		select {
		case err := <-ec:
			v.log(ca, roomID).WithError(err).Error("Error trying to validate if room exists")
			errors <- errCreateRoomCheckIfExistsFails
		case exists := <-rc:
			if exists {
				errors <- errCreateRoomAlreadyExists
				return
			}
			result <- true
		case <-v.cancel:
		}
	}()
}

func (a *account) createRoom(roomID jid.Bare, onSuccess func(), onError func(error)) {
	result := a.session.CreateRoom(roomID)
	go func() {
		err := <-result
		if err != nil {
			onError(err)
			return
		}
		onSuccess()
	}()
}

func (v *mucCreateRoomView) log(ca *account, roomID jid.Bare) coylog.Logger {
	l := v.u.log
	if ca != nil {
		l = ca.log
	}

	if roomID != nil {
		l.WithField("room", roomID)
	}

	l.WithField("who", "mucCreateRoomView")

	return l
}

func (v *mucCreateRoomView) createRoom(ca *account, roomID jid.Bare, errors chan error) {
	sc := make(chan bool)
	er := make(chan error)

	v.cancel = make(chan bool, 1)

	go func() {
		v.checkIfRoomExists(ca, roomID, sc, er)
		select {
		case <-sc:
			ca.createRoom(roomID, func() {
				v.onCreateRoomFinished(ca, roomID)
			}, func(err error) {
				v.log(ca, roomID).WithError(err).Error("Something went wrong while trying to create the room")
				errors <- errCreateRoomFailed
			})
		case err := <-er:
			errors <- err
		case <-v.cancel:
		}
	}()
}

func (v *mucCreateRoomView) onCreateRoomFinished(ca *account, roomID jid.Bare) {
	if v.autoJoin {
		doInUIThread(func() {
			v.joinRoom(ca, roomID)
		})
		return
	}

	doInUIThread(func() {
		v.showSuccessView(ca, roomID)
		v.dialog.ShowAll()
	})
}

// joinRoom MUST be called from the UI thread
func (v *mucCreateRoomView) joinRoom(ca *account, roomID jid.Bare) {
	v.dialog.Destroy()
	v.u.joinRoom(ca, roomID, nil)
}

func (v *mucCreateRoomView) updateAutoJoinValue(f bool) {
	if v.autoJoin == f {
		return
	}

	v.Lock()
	defer v.Unlock()

	v.autoJoin = f
	v.onAutoJoin.invokeAll()
}

func (u *gtkUI) mucCreateChatRoom() {
	view := newCreateMUCRoomView(u)

	u.connectShortcutsChildWindow(view.dialog)

	view.showCreateForm()

	view.dialog.SetTransientFor(u.window)
	view.dialog.Show()
}
