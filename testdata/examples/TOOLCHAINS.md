# Installing the example toolchains

Each example in this folder ships with a committed coverage report, so you
can browse it in CodeDiver and run `go test ./internal/core -run Example`
without installing anything here. Install an example's toolchain only to:

- **Run coverage** on it from CodeDiver's QA view
- run **Run these tests** (per-test coverage, Go and pytest only)
- regenerate its committed report after changing it (see the
  [README](README.md#regenerating-coverage))

| Example | Needs |
|---|---|
| go-service | Go |
| ts-react | Node.js |
| python-api | Python |
| java-maven | a JDK and Maven |
| rust-cargo | Rust (rustup) and cargo-llvm-cov |
| dotnet-xunit | the .NET SDK |
| ruby-rspec | Ruby and Bundler |
| polyglot-monorepo | Go, Node.js and Python |

The versions the committed reports came from are listed in the
[README](README.md#toolchains). Newer versions work too.

## Install

Each section gives a package-manager route per OS and a route that needs
no administrator rights (everything goes in your home folder). On Linux,
the no-admin routes keep your shell profile untouched unless you choose to
add the folder to `PATH`.

### Go

- **macOS:** `brew install go`
- **Windows:** `winget install GoLang.Go`
- **Linux, or without admin rights:** download the archive from
  [go.dev/dl](https://go.dev/dl/), then
  `mkdir -p ~/.local && tar -C ~/.local -xzf go*.linux-amd64.tar.gz`.
  Go is then in `~/.local/go/bin`.
- **Check:** `go version`

### Node.js (ts-react, the monorepo's `web`)

- **macOS:** `brew install node`
- **Windows:** `winget install OpenJS.NodeJS.LTS`
- **Linux, or without admin rights:** [nvm](https://github.com/nvm-sh/nvm),
  then `nvm install --lts`
- **Check:** `node --version && npm --version`
- **Per example:** run `npm install` in the example's folder before
  Run coverage. Without it, Vitest stops with "Cannot find package
  'vitest'".

### Python (python-api, the monorepo's `jobs`)

- **macOS:** `brew install python`
- **Windows:** `winget install Python.Python.3.12`
- **Linux:** usually installed; add `python3-venv` if `python3 -m venv`
  fails
- **Per example:** a virtual environment in the example's folder
  (git-ignored):

  ```sh
  python3 -m venv .venv
  . .venv/bin/activate          # Windows: .venv\Scripts\activate
  pip install -r requirements-dev.txt
  ```

  CodeDiver's suite runs `python -m pytest`. Start CodeDiver from the
  activated shell, or change the suite's command to the venv's Python
  (`.venv/bin/python -m pytest …`).

### Java and Maven (java-maven)

- **macOS:** `brew install openjdk maven`
- **Windows:** `winget install EclipseAdoptium.Temurin.21.JDK`, then Maven
  from [maven.apache.org](https://maven.apache.org/download.cgi) (unzip it
  and add its `bin` to `PATH`)
- **Linux, or without admin rights:** [SDKMAN!](https://sdkman.io), then
  `sdk install java` and `sdk install maven`
- **Check:** `java -version && mvn -version`
- **Per example:** Maven downloads JUnit and JaCoCo on the first run.

### Rust and cargo-llvm-cov (rust-cargo)

- **Every OS:** [rustup](https://rustup.rs). It installs into your home
  folder, so it never needs admin rights. On Linux and macOS:

  ```sh
  curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --profile minimal
  rustup component add llvm-tools-preview
  ```

  Add `--no-modify-path` to keep rustup from editing your shell profile;
  cargo is then in `~/.cargo/bin`. On Windows, use
  `winget install Rustlang.Rustup` or the installer from rustup.rs.
- **cargo-llvm-cov:** `cargo install cargo-llvm-cov`, or unpack a prebuilt
  binary from its
  [releases](https://github.com/taiki-e/cargo-llvm-cov/releases) into
  `~/.cargo/bin`
- **Check:** `cargo --version && cargo llvm-cov --version`

### .NET SDK (dotnet-xunit)

- **macOS:** `brew install --cask dotnet-sdk`
- **Windows:** `winget install Microsoft.DotNet.SDK.8`
- **Linux, or without admin rights:** Microsoft's install script, into
  `~/.dotnet`:

  ```sh
  curl -sSL https://dot.net/v1/dotnet-install.sh -o dotnet-install.sh
  bash dotnet-install.sh --channel 8.0 --install-dir ~/.dotnet --no-path
  export DOTNET_ROOT=~/.dotnet PATH=~/.dotnet:$PATH
  ```
- **Check:** `dotnet --version`
- **Per example:** `dotnet test` restores xUnit and coverlet from NuGet on
  the first run.

### Ruby and Bundler (ruby-rspec)

- **macOS:** `brew install ruby`. Use Homebrew's Ruby, not the system one,
  which is too old.
- **Windows:** [RubyInstaller](https://rubyinstaller.org) (Ruby+Devkit)
- **Linux:** your package manager (`ruby-full` on Debian and Ubuntu), or
  [rbenv](https://github.com/rbenv/rbenv) with ruby-build. Building Ruby
  needs the OpenSSL and libyaml development headers.
- **Linux without admin rights or build headers:** Homebrew's prebuilt
  [portable Ruby](https://github.com/Homebrew/homebrew-portable-ruby/releases).
  Unpack `portable-ruby-<version>.x86_64_linux.bottle.tar.gz` into
  `~/.local/ruby`; Ruby is then in `~/.local/ruby/portable-ruby/<version>/bin`.
  Install gems into your home folder too:
  `export GEM_HOME=~/.local/ruby/gems BUNDLE_PATH=~/.local/ruby/gems`.
- **Check:** `ruby --version && bundle --version`
- **Per example:** run `bundle install` in the example's folder before
  Run coverage.

## Making the tools visible to CodeDiver

**Run coverage** runs each suite's command in the system shell, with the
environment CodeDiver was started with:

- **Started from a terminal** (`codediver`, `codediver serve`): it sees
  that terminal's `PATH`, including anything added by nvm, rustup, SDKMAN!
  or an activated virtual environment.
- **Started from the desktop** (Dock, Start menu, app launcher): it may not
  see tools that only your shell profile adds. If a run reports "command
  not found", start CodeDiver from a terminal, or edit the suite in the QA
  view to use the tool's full path (for example
  `~/.cargo/bin/cargo llvm-cov …`).

Without admin rights on Linux, add the folders you installed to `PATH` in
the terminal you start CodeDiver from. Keep only the lines for the tools
you installed:

```sh
export PATH=~/.local/go/bin:~/.cargo/bin:~/.dotnet:~/.local/ruby/portable-ruby/<version>/bin:~/.local/ruby/gems/bin:$PATH
export DOTNET_ROOT=~/.dotnet GEM_HOME=~/.local/ruby/gems BUNDLE_PATH=~/.local/ruby/gems
```

## What not to commit

Installed dependencies and build output are git-ignored under
`testdata/examples`:
- `node_modules/`, `package-lock.json` and HTML coverage reports
- Rust `target/`, .NET `bin/` and `obj/`
- Bundler's `.bundle/` and SimpleCov's `.resultset.json`
- Python's `.venv/`

Only commit the coverage report files listed in the
[README](README.md).
