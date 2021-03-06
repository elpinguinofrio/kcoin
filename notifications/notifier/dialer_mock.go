// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package notifier

import (
	"github.com/go-mail/mail"
	"sync"
)

var (
	lockDialerMockDial sync.RWMutex
)

// DialerMock is a mock implementation of Dialer.
//
//     func TestSomethingThatUsesDialer(t *testing.T) {
//
//         // make and configure a mocked Dialer
//         mockedDialer := &DialerMock{
//             DialFunc: func() (mail.SendCloser, error) {
// 	               panic("TODO: mock out the Dial method")
//             },
//         }
//
//         // TODO: use mockedDialer in code that requires Dialer
//         //       and then make assertions.
//
//     }
type DialerMock struct {
	// DialFunc mocks the Dial method.
	DialFunc func() (mail.SendCloser, error)

	// calls tracks calls to the methods.
	calls struct {
		// Dial holds details about calls to the Dial method.
		Dial []struct {
		}
	}
}

// Dial calls DialFunc.
func (mock *DialerMock) Dial() (mail.SendCloser, error) {
	if mock.DialFunc == nil {
		panic("DialerMock.DialFunc: method is nil but Dialer.Dial was just called")
	}
	callInfo := struct {
	}{}
	lockDialerMockDial.Lock()
	mock.calls.Dial = append(mock.calls.Dial, callInfo)
	lockDialerMockDial.Unlock()
	return mock.DialFunc()
}

// DialCalls gets all the calls that were made to Dial.
// Check the length with:
//     len(mockedDialer.DialCalls())
func (mock *DialerMock) DialCalls() []struct {
} {
	var calls []struct {
	}
	lockDialerMockDial.RLock()
	calls = mock.calls.Dial
	lockDialerMockDial.RUnlock()
	return calls
}
