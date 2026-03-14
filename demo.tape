# go-trpc demo — generates assets/demo.gif
# Run: make demo (builds gotrpc binary + runs vhs)

Output assets/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 960
Set Height 700
Set Padding 20
Set TypingSpeed 45ms
Set Theme "Dracula"

# Setup
Hide
Type "export PATH=/tmp/gotrpc-bin:$PATH && cd /tmp/myapp && clear"
Enter
Sleep 500ms
Show

# Step 1: Init
Type "# Scaffold a new project"
Enter
Enter
Sleep 1s

Type "gotrpc init"
Enter
Sleep 6s

# Step 2: Show structs
Hide
Type "clear"
Enter
Sleep 300ms
Show
Type "# Typed structs from the generated handler"
Enter
Enter
Sleep 1s
Hide
Type "bat --style=plain --line-range 10:20 api/main.go && echo"
Enter
Sleep 500ms
Show
Sleep 6s

# Step 3: Show handlers
Hide
Type "clear"
Enter
Sleep 300ms
Show
Type "# Register handlers on the router"
Enter
Enter
Sleep 1s
Hide
Type "bat --style=plain --line-range 22:34 api/main.go && echo"
Enter
Sleep 500ms
Show
Sleep 6s

# Step 4: Generate types
Hide
Type "clear"
Enter
Sleep 300ms
Show
Type "# Generate TypeScript types from Go"
Enter
Enter
Sleep 1s

Type "gotrpc generate ./api"
Enter
Sleep 6s

# Step 5: Show generated types
Hide
Type "clear"
Enter
Sleep 300ms
Show
Type "# Generated router.d.ts"
Enter
Enter
Sleep 1s
Hide
Type "head -29 generated/router.d.ts | bat --style=plain -l typescript --force-colorization && echo"
Enter
Sleep 500ms
Show
Sleep 6s

# Step 6: Client usage
Hide
Type "clear"
Enter
Sleep 300ms
Show
Type "# Use with @trpc/client — fully typed!"
Enter
Enter
Sleep 1s
Hide
Type "bat --style=plain -l typescript /tmp/gotrpc-demo.ts && echo"
Enter
Sleep 500ms
Show
Sleep 6s
