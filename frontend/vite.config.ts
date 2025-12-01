import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// Useful development-time proxying so the Vite dev server can forward API requests
// to the backend server (http://localhost:8083) and avoid CORS preflight entirely.
export default defineConfig(({ mode }) => ({
	plugins: [sveltekit()],
	server: {
		proxy: {
			// Proxy requests to /data and /authProcess to the backend server
			'/data': {
				target: 'http://localhost:8083',
				changeOrigin: true,
				secure: false
			},
			'/authProcess': {
				target: 'http://localhost:8083',
				changeOrigin: true,
				secure: false
			},
			'/meta': {
				target: 'http://localhost:8083',
				changeOrigin: true,
				secure: false
			},
			'/search': {
				target: 'http://localhost:8083',
				changeOrigin: true,
				secure: false
			},
			'/lookup': {
				target: 'http://localhost:8083',
				changeOrigin: true,
				secure: false
			},
			'/lookupData': {
				target: 'http://localhost:8083',
				changeOrigin: true,
				secure: false
			},
			'/computedId': {
				target: 'http://localhost:8083',
				changeOrigin: true,
				secure: false
			}
		}
	}
}));
