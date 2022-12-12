<p align="center">
  <h3 align="center">Shoplazza CLI</h3>
  <p align="center">Shoplazza CLI is a command-line interface tool that helps you build Shoplazza themes. It quickly generates Shoplazza themes. You can also use it to automate many common development tasks.</p>
  <p align="center">
    <img src="https://img.shields.io/npm/v/shoplazza-cli">
  </p>
</p>

---

Shoplazza CLI is a cross-platform command line tool that you can use to build Shoplazza themes.

## Features

Shoplazza CLI accelerates your theme development process with the following features:

- Safely preview, test, and share changes to themes using unpublish themes
- Hot reload CSS and section changes, or automatically refresh a page on file change, when previewing a theme.
- Initialize a new theme using LifeStyle as a starting point.
- Use workflow tools like Git to work with a team of theme developers.
- Upload themes to your store.
- Watch for local changes and upload them automatically to Shoplazza.
- Work on Linux, macOS, and Windows.

## Installation [shoplazza-cli](https://www.npmjs.com/package/shoplazza-themekit)

```terminal
$ npm install shoplazza-cli -g
```

## Before you start

Before you start using Shoplazza CLI to develop themes, make sure that you do the following tasks:

- Install [Node.js](https://nodejs.org/en/) (14.14.0 or higher).
- Install [Git](https://git-scm.com/downloads).
- Make sure that you have a account with the Manage themes permission for the store that you want to work on, or you're the owner of the store.
- Note the URL of the store that you want to work on.
- Make sure that you're connected to the internet. Most Shoplazza CLI commands need an internet connection to run.

## Getting started

### Authenticate

```terminal
$ shoplazza login --store developer.myshoplaza.com
```

> In your browser window, log into the account that's attached to the store that you want to use for development.

### Create a new theme

```terminal
$ shoplazza theme init
```

> Use `shoplazza theme init` to create a new theme on your local machine. This command clones a Git repository to your local machine to use as the starting point for building a theme.

### Connect to existing theme

```terminal
$ shoplazza theme pull
```

> Pull the theme onto your local machine using `shoplazza theme pull`. You're prompted to select a theme from the list of themes on the store.

### Preview, test, and share your theme

```terminal
$ shoplazza theme serve
```

> After you create or navigate to your theme, you can run `shoplazza theme serve` to interact with the theme in a browser.

### Push your theme to your store

```terminal
$ shoplazza theme push
```

> Use `shoplazza theme push` to upload your local theme files to Shoplazza, overwriting the remote versions.

### Publish your theme

```terminal
$ shoplazza theme publish
```

> Use `shoplazza theme publish` to select and publish an unpublished theme from your theme library. If you want to publish your local theme, then you need to run `shoplazza theme push` first.

### Find your theme ID

```terminal
$ shoplazza theme list
```

> You might want to use a theme's ID to pull, push, publish, or delete a theme using Shoplazza CLI.

## Core commands

### help

```terminal
$ shoplazza help
```

> Lists the available commands and describes what they do.

### login

```terminal
$ shoplazza login --store developer.myshoplaza.com

$ shoplazza login --partner
```

> --store: Authenticates and logs you into the specified store with Shoplazza CLI.
> --partner: Authenticates and logs you into the partner with Shoplazza CLI.

### logout

```terminal
$ shoplazza logout

$ shoplazza logout --partner
```

> Default to logout your store, logs you out of the Shoplazza account and store, the logout command clears credentials. You need to reauthenticate the next time that you connect to a store.
> --partner: Clear your partner info, and you need to reauthenticate the next time.


### switch

```terminal
$ shoplazza switch --store developer.myshoplaza.com
```

> Switches between stores without logging out and logging in again.

### store

```terminal
$ shoplazza store
```

> Displays the store that you're currently connected to.

### version

```terminal
$ shoplazza version
```

> Displays the version of Shoplazza CLI that you're running.

## Theme commands

### init

```terminal
$ shoplazza theme init [--name]
```

> Clones a Git repository to your local machine to use as the starting point for building a theme.

### serve

```terminal
$ shoplazza theme serve [--theme]
```

> Uploads the current theme to the store that you're connected to, and returns the preview link.

### list

```terminal
$ shoplazza theme list
```

> Lists the themes in your store, along with their IDs and statuses.

### pull

```terminal
$ shoplazza theme pull [--theme]
```

> Retrieves theme files from Shoplazza, if no theme is specified, then you're prompted to select the theme to pull from the list of the themes in your store.

### push

```terminal
$ shoplazza theme push [--theme]
```

> Uploads your local theme files to Shoplazza, overwriting the remote theme if specified, if no theme is specified, then you're prompted to select the theme to overwrite from the list of the themes in your store.

### share

```terminal
$ shoplazza theme share
```

> Uploads your theme as a new, unpublished theme in your theme library. The command return a preview link that you can share with others.

### publish

```terminal
$ shoplazza theme publish [--theme]
```

> Publishes an unpublished theme from your theme library, if no theme ID is specified, then you're prompted to select the theme that you want to publish from the list of themes in your store.

### package

```terminal
$ shoplazza theme package
```

> Packages your local theme files into a ZIP file that can be uploaded to Shoplazza. The ZIP file uses the name theme_name-theme_version.zip, based on parameters in your `settings_schema.json` file.

### delete

```terminal
$ shoplazza theme delete [--theme]
```

> Deletes a theme from your store, if no theme is specified, then you're prompted to select the theme that you want to delete from the list of themes in your store.

## app

### generate

```terminal
$ shoplazza app generate extension
```

> Choose Your app type and download template.

### build

```terminal
$ shoplazza app build
```

> Minify css / js files and generate manifest.json and zip.

### deploy

```terminal
$ shoplazza app deploy extension
```

> Deploy your zip to oss for publish.

### pupblish

```terminal
$ shoplazza app publish extension
```

> Choose your extension version and publish to your store.

## Theme Directory

You can run certain theme commands, such as shoplazza theme serve, only if the directory you're using matches the default Shoplazza theme directory structure. This structure represents a buildless theme, or a theme that has already gone through any necessary file transformations. If you use build tools to generate theme files, then you might need to run commands from the directory where the generated files are stored.

The default Shoplazza theme directory structure is as follows:

```terminal
└── project
    ├── assets
    ├── config
    ├── layout
    ├── locales
    ├── sections
    ├── snippets
    └── templates
```

## Support(OS Terminals)

You should expect mostly good support for the CLI below. This does not mean we won't look at issues found on other command line - feel free to report any!

- Mac OS
  - Terminal.app
  - iTerm
- Windows (Known issues):
  - ConEmu
  - cmd.exe
  - Powershell
  - Cygwin
- Linux (Ubuntu, openSUSE, Arch Linux, etc):
  - gnome-terminal (Terminal GNOME)
  - konsole
