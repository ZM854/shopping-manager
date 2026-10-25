// @vitest-environment node
import SwaggerParser from '@apidevtools/swagger-parser';
import type { OpenAPIV3 } from 'openapi-types';
import { fileURLToPath } from 'node:url';
import { beforeAll, describe, expect, it } from 'vitest';

type NextContract = {
  version: string;
  description: string;
  status: string;
  paths: OpenAPIV3.PathsObject;
  limits: Record<string, number | number[]>;
  replayFormats: Record<string, string>;
};
type Contract = OpenAPIV3.Document & { 'x-next-contract'?: NextContract };

const contractPath = fileURLToPath(
  new URL('../../../backend/api/openapi.yaml', import.meta.url),
);
const mediaPaths = [
  '/recipe-media',
  '/recipe-media/{mediaId}',
  '/recipe-media/{mediaId}/content',
  '/recipe-media/access',
];
let active: Contract;
let target: Contract;
let next: NextContract;

function schema(document: Contract, name: string): OpenAPIV3.SchemaObject {
  const value = document.components?.schemas?.[name];
  if (!value || '$ref' in value) throw new Error(`Нет схемы ${name}`);
  return value;
}

function body(operation: OpenAPIV3.OperationObject | undefined) {
  const value = operation?.requestBody;
  if (!value || '$ref' in value) throw new Error('Нет тела запроса');
  return value;
}

function response(
  operation: OpenAPIV3.OperationObject | undefined,
  status: string,
) {
  const value = operation?.responses?.[status];
  if (!value || '$ref' in value) throw new Error(`Нет ответа ${status}`);
  return value;
}

beforeAll(async () => {
  // Parse first: validate/dereference mutate objects, including shared $refs.
  const parsed = await SwaggerParser.parse(contractPath);
  if (!('openapi' in parsed) || parsed.openapi !== '3.0.3')
    throw new Error('Ожидался OpenAPI 3.0.3');
  const document = parsed as Contract;
  if (!document['x-next-contract']) throw new Error('Нет целевого контракта');
  next = structuredClone(document['x-next-contract']);
  const draft = structuredClone(document);
  draft.info.version = next.version;
  draft.info.description = next.description;
  draft.paths = { ...draft.paths, ...next.paths };
  delete draft['x-next-contract'];
  [active, target] = (await Promise.all([
    SwaggerParser.validate(structuredClone(document)),
    SwaggerParser.validate(draft),
  ])) as Contract[];
});

describe('Целевой HTTP-контракт медиа рецептов', () => {
  it('проверяет обе версии и сохраняет действующие маршруты/DTO до реализации', () => {
    expect(active.info.version).toBe('0.8.0');
    expect(next.status).toBe('planned');
    expect(target.info.version).toBe('0.9.0');
    for (const path of mediaPaths) {
      expect(active.paths[path]).toBeUndefined();
      expect(target.paths[path]).toBeDefined();
    }
    expect(schema(active, 'RecipeStep').properties).not.toHaveProperty('id');
    expect(schema(active, 'RecipeSummary').properties).not.toHaveProperty(
      'cover',
    );
    for (const [path, value] of Object.entries(active.paths)) {
      if (path === '/recipes' || path === '/recipes/{recipeId}') continue;
      expect(target.paths[path]).toEqual(value);
    }
  });

  it('защищает все целевые операции и ошибки, не кэширует подписи', () => {
    const operations: string[] = [];
    for (const [path, entry] of Object.entries(target.paths)) {
      if (!path.startsWith('/recipes') && !path.startsWith('/recipe-media'))
        continue;
      for (const method of ['get', 'post', 'put', 'patch', 'delete'] as const) {
        const operation = entry?.[method];
        if (!operation) continue;
        operations.push(`${method.toUpperCase()} ${path}`);
        expect(operation.security).toEqual([{ bearerAuth: [] }]);
        expect(operation.responses['401']).toBeDefined();
        expect(operation.responses['403']).toBeDefined();
        for (const value of Object.values(operation.responses)) {
          if ('$ref' in value) throw new Error('Не разрешена ссылка ответа');
          expect(value.headers?.['Cache-Control']).toMatchObject({
            schema: { enum: ['no-store'] },
          });
        }
      }
    }
    expect(operations.sort()).toEqual(
      [
        'GET /recipes',
        'POST /recipes',
        'GET /recipes/{recipeId}',
        'PUT /recipes/{recipeId}',
        'DELETE /recipes/{recipeId}',
        'POST /recipe-media',
        'GET /recipe-media/{mediaId}',
        'DELETE /recipe-media/{mediaId}',
        'PUT /recipe-media/{mediaId}/content',
        'POST /recipe-media/access',
      ].sort(),
    );
    const access = target.paths['/recipe-media/access']?.post;
    expect(access?.parameters).toBeUndefined();
    expect(
      schema(target, 'RecipeMediaAccessRequest').properties?.items,
    ).toMatchObject({
      minItems: 1,
      maxItems: next.limits.accessBatchSize,
      uniqueItems: true,
    });
    expect(next.limits.accessTtlSeconds).toBe(300);
  });

  it('отделяет бинарную загрузку и её повторы от небольших JSON-запросов', () => {
    const reserve = target.paths['/recipe-media']?.post;
    const upload = target.paths['/recipe-media/{mediaId}/content']?.put;
    expect(reserve?.parameters).toContainEqual(
      expect.objectContaining({
        name: 'Idempotency-Key',
        in: 'header',
        required: true,
      }),
    );
    expect(upload?.parameters).toBeUndefined();
    expect(body(upload).content).toEqual({
      'application/octet-stream': {
        schema: { type: 'string', format: 'binary' },
      },
    });
    expect(body(upload)).toMatchObject({
      'x-max-body-bytes': next.limits.videoInputBytes,
      'x-max-image-body-bytes': next.limits.imageInputBytes,
    });
    expect(body(reserve)).toHaveProperty('x-max-body-bytes', 4096);
    expect(body(target.paths['/recipe-media/access']?.post)).toHaveProperty(
      'x-max-body-bytes',
      16384,
    );
    for (const [name, limit] of [
      ['ReserveRecipeImage', next.limits.imageInputBytes],
      ['ReserveRecipeVideo', next.limits.videoInputBytes],
    ] as const) {
      expect(schema(target, name)).toMatchObject({
        additionalProperties: false,
        required: ['purpose', 'byteSize', 'sha256'],
        properties: {
          byteSize: { minimum: 1, maximum: limit },
          sha256: { pattern: '^[a-f0-9]{64}$' },
        },
      });
    }
    for (const status of ['409', '413', '415', '422', '429', '503', '504']) {
      expect(response(upload, status)).toBeDefined();
    }
    for (const status of ['409', '429']) {
      expect(response(upload, status).headers?.['Retry-After']).toMatchObject({
        schema: { type: 'integer', minimum: 1 },
      });
    }
    expect(
      response(target.paths['/recipe-media/{mediaId}']?.delete, '204'),
    ).not.toHaveProperty('content');
  });

  it('сохраняет полную замену PUT и запрещает присваивать ID шагу через POST', () => {
    expect(schema(target, 'CreateRecipeWithMedia').required).not.toContain(
      'coverMediaId',
    );
    expect(schema(target, 'UpdateRecipeWithMedia').required).toContain(
      'coverMediaId',
    );
    expect(
      schema(target, 'RecipeCreateStepWithMedia').properties,
    ).not.toHaveProperty('id');
    expect(schema(target, 'RecipeUpdateStepWithMedia')).toMatchObject({
      additionalProperties: false,
      required: ['text', 'mediaIds'],
      properties: {
        id: { type: 'string', maxLength: 19 },
        mediaIds: { maxItems: next.limits.stepAttachments, uniqueItems: true },
      },
    });
    for (const name of ['CreateRecipeWithMedia', 'UpdateRecipeWithMedia']) {
      const value = schema(target, name);
      expect(value.additionalProperties).toBe(false);
      expect(value.properties?.coverMediaId).toMatchObject({
        type: 'string',
        nullable: true,
      });
      for (const forbidden of [
        'id',
        'ownerId',
        'recipeId',
        'position',
        'createdAt',
      ]) {
        expect(value.properties).not.toHaveProperty(forbidden);
      }
    }
    for (const operation of [
      target.paths['/recipes']?.post,
      target.paths['/recipes/{recipeId}']?.put,
    ]) {
      expect(body(operation)).toHaveProperty('x-max-body-bytes', 1048576);
    }
    expect(next.limits.recipeAttachments).toBe(20);
    expect(next.limits.stepVideos).toBe(1);
    expect(next.limits.recipeVideos).toBe(5);
  });

  it('оставляет старые снимки только у POST replay, без подписанных URL в DTO', () => {
    const created = response(target.paths['/recipes']?.post, '201').content?.[
      'application/json'
    ].schema;
    expect(created).toHaveProperty('oneOf', [
      schema(target, 'Recipe'),
      schema(target, 'RecipeWithStepIds'),
      schema(target, 'RecipeWithMedia'),
    ]);
    expect(Object.keys(next.replayFormats)).toEqual(['1', '2', '3']);
    for (const operation of [
      target.paths['/recipes/{recipeId}']?.get,
      target.paths['/recipes/{recipeId}']?.put,
    ]) {
      expect(
        response(operation, '200').content?.['application/json'].schema,
      ).toEqual(schema(target, 'RecipeWithMedia'));
    }
    for (const name of [
      'RecipeCoverMedia',
      'RecipeImageMedia',
      'RecipeVideoMedia',
    ]) {
      expect(Object.keys(schema(target, name).properties ?? {}).sort()).toEqual(
        ['id', 'purpose', 'variants'],
      );
    }
    expect(schema(target, 'RecipeWithMedia').required).toContain('cover');
    expect(schema(target, 'RecipeStepWithMedia').required).toEqual([
      'id',
      'position',
      'text',
      'media',
    ]);
    expect(schema(target, 'RecipeNullableCover').nullable).toBe(true);
    expect(schema(target, 'RecipeMediaState').enum).toEqual([
      'awaiting_upload',
      'processing',
      'ready',
      'attached',
      'failed',
      'delete_pending',
    ]);
  });
});
