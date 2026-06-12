# Theme Basic App

## Overview

Basic Extension (a card that allows store owners to decide where to add it)

## Installation

First, ensure you have "Shoplazza CLI" installed. If not, you can install it globally using the following command:

```bash
npm install -g shoplazza-cli
```

## Available Scripts

### `create`

Create a theme Extension project

```bash
shoplazza te create
```

---

After entering the project directory, you can use the following commands:

### `start`

Start development mode

```bash
npm start
```

or

```bash
shoplazza te serve
```

### `build`

Build a production version of the current Extension

```bash
npm run build
```

or

```bash
shoplazza te build
```

### `deploy`

Deploy a production version of an Extension to the current store

```bash
npm run deploy
```

or

```bash
shoplazza te deploy
```

### `versions`

View the production version list of an Extension

```bash
npm run versions
```

or

```bash
shoplazza te versions
```

### `list`

Query the list of private Extensions in the store

```bash
npm run list
```

or

```bash
shoplazza te list
```

### `connect`

Bind an Extension to a specific APP

```bash
npm run connect
```

or

```bash
shoplazza te connect
```

### `release`

Release a production version of an Extension to the bound APP

```bash
npm run release
```

or

```bash
shoplazza te release
```