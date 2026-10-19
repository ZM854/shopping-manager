// @vitest-environment node
import SwaggerParser from '@apidevtools/swagger-parser';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const contractPath = fileURLToPath(
  new URL('../../../backend/api/openapi.yaml', import.meta.url),
);

describe('OpenAPI списков', () => {
  it('имеет валидную структуру и не зависит от внешних файлов', async () => {
    const parser = new SwaggerParser();
    const document = await parser.validate(contractPath);
    expect(document.info.version).toBe('0.4.0');
    expect(parser.$refs.paths()).toHaveLength(1);
  });

  it('описывает все shopping-маршруты с авторизацией без старых products', async () => {
    const document = await SwaggerParser.validate(contractPath);
    expect(document.paths?.['/products']).toBeUndefined();
    expect(document.paths?.['/users']).toBeUndefined();
    expect(document.paths?.['/products/{id}']).toBeUndefined();
    let operations = 0;
    for (const [path, entry] of Object.entries(document.paths ?? {})) {
      if (!path.startsWith('/shopping-lists')) continue;
      for (const method of ['get', 'post', 'patch', 'delete'] as const) {
        const operation = entry?.[method];
        if (!operation) continue;
        operations++;
        expect(operation.security).toEqual([{ bearerAuth: [] }]);
        expect(operation.responses?.['401']).toBeDefined();
      }
    }
    expect(operations).toBe(12);
  });

  it('сохраняет доступ к публичным маршрутам и авторизацию по refresh cookie', async () => {
    const document = await SwaggerParser.validate(contractPath);
    expect(document.security).toBeUndefined();
    for (const path of [
      '/healthz',
      '/registration',
      '/login',
      '/activation',
      '/activation/resend',
    ]) {
      expect(document.paths?.[path]).toBeDefined();
    }
    for (const path of ['/refresh', '/logout']) {
      expect(document.paths?.[path]?.post?.security).toEqual([
        { refreshCookie: [] },
      ]);
    }
    expect(document.paths?.['/activate/{link}']).toBeUndefined();
    expect(
      document.paths?.['/activation']?.post?.responses?.['204'],
    ).toBeDefined();
    expect(
      document.paths?.['/activation/resend']?.post?.responses?.['429'],
    ).toBeDefined();
    expect(
      document.paths?.['/registration']?.post?.responses?.['503'],
    ).toBeDefined();
  });

  it('разделяет регистрацию и безопасную сессию без refresh в JSON', async () => {
    const document = await SwaggerParser.dereference(contractPath);
    if (!('components' in document)) throw new Error('Ожидался OpenAPI 3');
    const schemas = document.components?.schemas;
    expect(schemas?.RegistrationResponse).toMatchObject({
      additionalProperties: false,
      required: ['activationRequired'],
      properties: { activationRequired: { type: 'boolean', enum: [true] } },
    });
    const response = schemas?.AuthResponse;
    if (!response || !('properties' in response))
      throw new Error('Нет AuthResponse');
    expect(response.additionalProperties).toBe(false);
    expect(Object.keys(response.properties ?? {}).sort()).toEqual([
      'accessToken',
      'user',
    ]);
    for (const path of ['/login', '/refresh']) {
      expect(document.paths?.[path]?.post?.responses?.['403']).toBeDefined();
      expect(document.paths?.[path]?.post?.responses?.['500']).toBeDefined();
    }
  });

  it('фиксирует границы decimal-строки', async () => {
    const document = await SwaggerParser.dereference(contractPath);
    if (!('components' in document)) {
      throw new Error('Ожидался OpenAPI 3 с components');
    }
    const schema = document.components?.schemas?.Quantity;
    if (!schema || !('pattern' in schema) || !schema.pattern) {
      throw new Error('В контракте отсутствует Quantity.pattern');
    }
    const pattern = new RegExp(schema.pattern);
    for (const value of ['0.001', '0.125', '1', '1.000', '999999999.999']) {
      expect(pattern.test(value), value).toBe(true);
    }
    for (const value of [
      '0',
      '0.000',
      '-1',
      'NaN',
      'Infinity',
      '1e3',
      '1,5',
      '01',
      '.5',
      '1.0001',
      '1000000000',
      ' 1 ',
    ]) {
      expect(pattern.test(value), value).toBe(false);
    }
  });
});
