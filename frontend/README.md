# sv

Everything you need to build a Svelte project, powered by [`sv`](https://github.com/sveltejs/cli).

## Creating a project

If you're seeing this, you've probably already done this step. Congrats!

```sh
# create a new project in the current directory
npx sv create

# create a new project in my-app
npx sv create my-app
```

## Developing

Once you've created a project and installed dependencies with `npm install` (or `pnpm install` or `yarn`), start a development server:

```sh
npm run dev

# or start the server and open the app in a new browser tab
npm run dev -- --open
```

### Chat prototype

The default route (`/`) now renders a simple threaded chat sandbox. Every message you submit becomes a child of the previously submitted message, producing a cascading conversation tree. Use the textarea at the bottom of the page and press `Enter` (or click **Send**) to append a reply. Hold `Shift + Enter` to insert newlines without sending.

### Backend integration

The chat client persists every message to the Ouroboros API via the `/data` endpoint. While a message is being written, the UI shows a **Saving…** badge next to the entry and disables the send button. Once the backend acknowledges the write, the saved hash appears alongside the message and a success banner confirms the operation. Network or server failures surface inline errors.

By default the client targets `http://localhost:8083`. Override the destination by setting `VITE_OUROBOROS_API` before starting the dev server:

```sh
VITE_OUROBOROS_API="http://your-api-host:8083" npm run dev
```

Ensure the Go server is running before interacting with the chat UI so saves can complete successfully.

## Building

To create a production version of your app:

```sh
npm run build
```

You can preview the production build with `npm run preview`.

> To deploy your app, you may need to install an [adapter](https://svelte.dev/docs/kit/adapters) for your target environment.
