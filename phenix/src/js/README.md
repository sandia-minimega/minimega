# phenixweb

Vue.js single-page application leveraging Vuex, vue-router, and
vue-resource.

## Project Setup

### Install Runtimes

If using [asdf-vm](https://github.com/asdf-vm) to manage runtimes, run
the following to install the correct version of Node.

```
asdf install
```

### Install Project's Node dependencies

```
npm install
```

### Run Development Servers

In one terminal, compile and run Vue.js development server (with
hot-reload):

```
npm run serve
```

We no longer use the json-server.

In another terminal, run mock API server with the following command:

```
npx json-server --watch db.json
```

On macOS, this worked:

```
json-server --watch db.json
```

### Compile and Minify for Production

```
npm run build
```

### Lint and Fix Files

```
npm run lint
```
