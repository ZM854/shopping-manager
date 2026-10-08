import { beforeEach, expect, it, vi } from 'vitest';
beforeEach(() => {
  vi.resetModules();
});
it.each(['dev', 'prod'])('логирование в %s', async (environment) => {
  vi.stubEnv('VITE_ENV', environment);
  const log = vi.spyOn(console, 'log').mockImplementation(() => {}),
    warn = vi.spyOn(console, 'warn').mockImplementation(() => {}),
    error = vi.spyOn(console, 'error').mockImplementation(() => {});
  const { logger } = await import('./logger');
  logger.info('TEST', 'info');
  logger.debug('TEST', 'debug');
  logger.warn('TEST', 'warn');
  logger.error('TEST', 'error', new Error('test'));
  expect(log).toHaveBeenCalledTimes(environment === 'dev' ? 2 : 0);
  expect(warn).toHaveBeenCalledTimes(environment === 'dev' ? 1 : 0);
  expect(error).toHaveBeenCalledOnce();
});
