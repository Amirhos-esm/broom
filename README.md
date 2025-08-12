# broom

`broom` is a Go package and utility for managing and cleaning up folders by automatically removing files based on configurable strategies. It helps keep disk usage under control by sweeping specified directories and deleting files according to customizable rules (e.g., oldest first, alphabetical order, etc.).

## Features

- Manage multiple folders with size limits.
- Customizable file removal strategies.
- Background cleanup process with configurable intervals.
- Thread-safe operation queue for folder management.
- Simple API to add, remove, and query managed folders.

## Installation

```bash
go get github.com/Amirhos-esm/broom
```

## Usage

```go
package main

import (
	"fmt"
	"time"

	"github.com/Amirhos-esm/broom"
)

func main() {
	// Create a new broom instance with a sweep interval of 10 minutes
	br := broom.NewBroom(10 * time.Minute)

	// Start the background cleanup process
	br.Run()
	defer br.Stop()

	// Add a folder to manage, with a max size of 1 GB (assuming Size is a defined type)
	err := br.AddFolder("/path/to/folder",broom.GByte * 1)
	if err != nil {
		fmt.Println("Error adding folder:", err)
		return
	}

	// Query folder info
	folder, err := br.GetFolder("/path/to/folder")
	if err != nil {
		fmt.Println("Error getting folder:", err)
		return
	}

	fmt.Printf("Folder info: %+v
", folder)

	// ... your application logic here ...
}
```

## Custom Removal Strategies

You can define your own file removal strategy by implementing the `RemovingStrategy` function signature:

```go
var MyStrategy broom.RemovingStrategy = func(folder *broom.BroomFolder, files []broom.File, needReduce broom.Size) []broom.File {
	// Your logic to select which files to remove
}
```

Assign your strategy to your broom instance:

```go
br.RemovingStrategy = MyStrategy
```

## Contributing

Contributions, issues, and feature requests are welcome! Feel free to check the [issues page](https://github.com/yourusername/broom/issues).

## License

This project is licensed under the MIT License.
