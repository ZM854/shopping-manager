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
    expect(document.info.version).toBe('0.7.0');
    expect(parser.$refs.paths()).toHaveLength(1);
  });

  it('описывает все shopping-маршруты с авторизацией без старых products', async () => {
    const document = await SwaggerParser.validate(contractPath);
    expect(document.paths?.['/products']).toBeUndefined();
    expect(document.paths?.['/users']).toBeUndefined();
    expect(document.paths?.['/products/{id}']).toBeUndefined();
    const operations: string[] = [];
    for (const [path, entry] of Object.entries(document.paths ?? {})) {
      if (
        !path.startsWith('/shopping-lists') &&
        !path.startsWith('/shopping-list-shares')
      )
        continue;
      for (const method of ['get', 'post', 'patch', 'delete'] as const) {
        const operation = entry?.[method];
        if (!operation) continue;
        operations.push(`${method.toUpperCase()} ${path}`);
        expect(operation.security).toEqual([{ bearerAuth: [] }]);
        expect(operation.responses?.['401']).toBeDefined();
      }
    }
    expect(operations.sort()).toEqual(
      [
        'GET /shopping-lists',
        'POST /shopping-lists',
        'GET /shopping-lists/{listId}',
        'PATCH /shopping-lists/{listId}',
        'DELETE /shopping-lists/{listId}',
        'GET /shopping-lists/{listId}/items',
        'POST /shopping-lists/{listId}/items',
        'DELETE /shopping-lists/{listId}/items',
        'PATCH /shopping-lists/{listId}/items/mark-all',
        'GET /shopping-lists/{listId}/items/{itemId}',
        'PATCH /shopping-lists/{listId}/items/{itemId}',
        'DELETE /shopping-lists/{listId}/items/{itemId}',
        'POST /shopping-lists/{listId}/shares',
        'POST /shopping-list-shares/import',
      ].sort(),
    );
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
    expect(document.paths?.['/refresh']?.post?.security).toEqual([
      { refreshCookie: [] },
    ]);
    expect(document.paths?.['/logout']?.post?.security).toEqual([
      {},
      { refreshCookie: [] },
    ]);
    expect(
      document.paths?.['/logout']?.post?.responses?.['401'],
    ).toBeUndefined();
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
    expect(schemas?.UserDTO).toMatchObject({
      properties: { id: { type: 'string', pattern: '^[1-9][0-9]{0,18}$' } },
    });
    for (const path of ['/login', '/registration']) {
      expect(document.paths?.[path]?.post?.responses?.['413']).toBeDefined();
      expect(document.paths?.[path]?.post?.responses?.['429']).toBeDefined();
    }
    for (const path of ['/refresh', '/logout']) {
      expect(document.paths?.[path]?.post?.parameters).toContainEqual(
        expect.objectContaining({
          in: 'header',
          name: 'Origin',
          required: true,
        }),
      );
    }
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

async function loadSharingContract() {
  const document = await SwaggerParser.validate(contractPath);
  if (!('components' in document)) throw new Error('Ожидался OpenAPI 3');
  return document;
}

const sharingOperations = [
  {
    path: '/shopping-lists/{listId}/shares',
    operationId: 'createShoppingListShare',
    requestSchema: 'CreateShoppingListShare',
    responseSchema: 'ShoppingListShare',
  },
  {
    path: '/shopping-list-shares/import',
    operationId: 'importShoppingListShare',
    requestSchema: 'ImportShoppingListShare',
    responseSchema: 'ShoppingList',
  },
];

describe('OpenAPI обмена списками', () => {
  it.each(sharingOperations)(
    '$operationId требует Bearer, ключ действия и JSON без секрета в URL',
    async ({ path, operationId, requestSchema }) => {
      const document = await loadSharingContract();
      const entry = document.paths?.[path];
      const operation = entry?.post;
      expect(operation).toMatchObject({
        operationId,
        security: [{ bearerAuth: [] }],
        requestBody: {
          required: true,
          content: {
            'application/json': {
              schema: document.components?.schemas?.[requestSchema],
            },
          },
        },
      });
      expect(entry?.get).toBeUndefined();
      expect(entry?.patch).toBeUndefined();
      expect(entry?.delete).toBeUndefined();
      expect(operation?.parameters).toEqual([
        expect.objectContaining({
          name: 'Idempotency-Key',
          in: 'header',
          required: true,
          schema: {
            type: 'string',
            minLength: 16,
            maxLength: 128,
            pattern: '^[A-Za-z0-9_-]{16,128}$',
          },
        }),
      ]);
      if (path.includes('{listId}')) {
        expect(entry?.parameters).toEqual([
          {
            name: 'listId',
            in: 'path',
            required: true,
            schema: document.components?.schemas?.Id,
          },
        ]);
      } else {
        expect(entry?.parameters).toBeUndefined();
      }
    },
  );

  it('требует секрет и явный boolean, запрещает передачу владельца и содержимого', async () => {
    const document = await loadSharingContract();
    const schemas = document.components?.schemas;
    expect(schemas?.CreateShoppingListShare).toEqual({
      type: 'object',
      additionalProperties: false,
      required: ['token', 'preserveMarks'],
      properties: {
        token: schemas?.ShareToken,
        preserveMarks: {
          type: 'boolean',
          description: expect.any(String),
        },
      },
    });
    expect(schemas?.ImportShoppingListShare).toEqual({
      type: 'object',
      additionalProperties: false,
      required: ['token'],
      properties: { token: schemas?.ShareToken },
    });
    expect(schemas?.ShareToken).toMatchObject({
      type: 'string',
      minLength: 43,
      maxLength: 43,
    });
    expect(schemas?.ShareToken).not.toHaveProperty('nullable', true);
  });

  it('принимает только канонический base64url секрета из 32 байт', async () => {
    const document = await loadSharingContract();
    const schema = document.components?.schemas?.ShareToken;
    if (!schema || !('pattern' in schema) || !schema.pattern)
      throw new Error('В контракте отсутствует ShareToken.pattern');
    const pattern = new RegExp(schema.pattern);
    const token = Buffer.from(
      Array.from({ length: 32 }, (_, index) => index),
    ).toString('base64url');
    for (const value of [
      token,
      Buffer.alloc(32).toString('base64url'),
      Buffer.alloc(32, 255).toString('base64url'),
    ]) {
      expect(pattern.test(value), value).toBe(true);
    }
    for (const value of [
      '',
      token.slice(1),
      `${token}A`,
      `${token}=`,
      ` ${token}`,
      `${token} `,
      `+${token.slice(1)}`,
      `/${token.slice(1)}`,
      `я${token.slice(1)}`,
      `${token.slice(0, -1)}B`,
    ]) {
      expect(pattern.test(value), value).toBe(false);
    }
    // У последнего символа base64url для 32 байт два младших бита равны нулю.
    const alphabet =
      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';
    for (const [index, character] of [...alphabet].entries()) {
      expect(pattern.test(`${token.slice(0, -1)}${character}`)).toBe(
        index % 4 === 0,
      );
    }
  });

  it.each(sharingOperations)(
    '$operationId возвращает безопасный DTO, единые ошибки и no-store при любом исходе',
    async ({ path, responseSchema }) => {
      const document = await loadSharingContract();
      const responses = document.paths?.[path]?.post?.responses;
      expect(Object.keys(responses ?? {}).sort()).toEqual([
        '201',
        '400',
        '401',
        '403',
        '404',
        '409',
        '413',
        '500',
      ]);
      for (const [status, response] of Object.entries(responses ?? {})) {
        expect(response).toMatchObject({
          headers: {
            'Cache-Control': {
              schema: { type: 'string', enum: ['no-store'] },
            },
          },
          content: {
            'application/json': {
              schema:
                document.components?.schemas?.[
                  status === '201' ? responseSchema : 'Error'
                ],
            },
          },
        });
        if ('$ref' in response) throw new Error('Ожидался раскрытый ответ');
        expect(response.headers?.['Set-Cookie']).toBeUndefined();
      }
      expect(responses?.['400']).toMatchObject({
        description: expect.stringContaining('validation_failed'),
      });
      expect(responses?.['413']).toMatchObject({
        description: expect.stringContaining('65536'),
      });
    },
  );

  it('отвечает метаданными снимка без секрета, URL и профиля автора', async () => {
    const document = await loadSharingContract();
    expect(document.components?.schemas?.ShoppingListShare).toEqual({
      type: 'object',
      additionalProperties: false,
      required: ['id', 'createdAt', 'preserveMarks'],
      properties: {
        id: document.components?.schemas?.Id,
        createdAt: { type: 'string', format: 'date-time' },
        preserveMarks: { type: 'boolean' },
      },
    });
    expect(document.components?.schemas?.ShoppingList).toMatchObject({
      additionalProperties: false,
      required: ['id', 'name', 'createdAt', 'updatedAt'],
      properties: { id: document.components?.schemas?.Id },
    });
  });

  it('различает повтор попытки и новое подтверждённое добавление после удаления копии', async () => {
    const document = await loadSharingContract();
    const description =
      document.paths?.['/shopping-list-shares/import']?.post?.description ?? '';
    expect(description).toContain(
      'только после явного подтверждения «Добавить»',
    );
    expect(description).toContain(
      '«Отмена» и открытие страницы не отправляют POST',
    );
    expect(description).toContain(
      'минимум 24 часа возвращают исходные 201/DTO',
    );
    expect(description).toContain(
      'Новый Idempotency-Key означает новое подтверждённое добавление',
    );
    expect(description).toContain('прежняя копия существует или удалена');
    expect(document.components?.responses?.SharingImportConflict).toMatchObject(
      {
        description: expect.stringContaining('idempotency_conflict'),
      },
    );
    expect(document.components?.responses?.SharingImportConflict).toMatchObject(
      {
        description: expect.stringContaining('resource_deleted'),
      },
    );
    expect(document.components?.responses?.SharingCreateConflict).toMatchObject(
      {
        description: expect.stringContaining('share_token_conflict'),
      },
    );
  });
});
