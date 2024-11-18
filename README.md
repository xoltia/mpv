# MPV IPC for Go
This is a Go package that provides an interface to the IPC mechanism
of the [mpv media player](https://mpv.io/).

## Installation
To install the package, run:
```sh
go get github.com/xoltia/mpv
```

## Requirements
The package requires that the `mpv` executable is installed on the system.

## Usage
Check the [documentation](https://pkg.go.dev/github.com/xoltia/mpv) for more information. For specific commands and properties not directly implemented, see the
[mpv IPC documentation](https://mpv.io/manual/master/#json-ipc) and
make use of the command and property functions directly.

Here is a simple example of playing a video:
```go
package main

import (
    "fmt"

    "github.com/xoltia/mpv"
)

func main() {
    m := mpv.NewMPVProcess()
    defer m.Close()
	
    c, err := m.OpenClient()
    if err != nil {
        panic(err)
    }
    defer c.Close()
	
    err = c.LoadFile("https://youtu.be/YKEhO5jhP3g", mpv.LoadFileModeReplace)
    if err != nil {
        panic(err)
    }
	
    err = c.Play()
    if err != nil {
        panic(err)
    }

    select {}
}
```
> [!NOTE]
> This example also requires that yt-dlp is installed on the system.

For a more complete example, see the [example](example) directory.
An IPC connection can be opened with an existing mpv process by
using the `OpenClient` function directly.
