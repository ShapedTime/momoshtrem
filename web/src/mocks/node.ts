import { setupServer } from 'msw/node';
import { handlers } from './handlers';

/** The same handlers for Jest (node environment). */
export const server = setupServer(...handlers);
