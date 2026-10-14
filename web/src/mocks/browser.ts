import { setupWorker } from 'msw/browser';
import { handlers } from './handlers';

/** Mock mode in the browser (see components/dev/MockProvider.tsx). */
export const worker = setupWorker(...handlers);
