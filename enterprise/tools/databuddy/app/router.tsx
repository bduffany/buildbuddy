import createRouter, { Route } from 'router5';
import browserPlugin from 'router5-plugin-browser';

const routes: Route<Record<string, any>>[] = [
  { name: 'home', path: '/' },
  { name: 'query', path: '/q/:id?sort&sortDir' },
];

const router = createRouter(routes);
router.usePlugin(browserPlugin());
router.start();

export default router;
