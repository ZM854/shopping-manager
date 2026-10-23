// @vitest-environment node
import SwaggerParser from '@apidevtools/swagger-parser';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const contractPath = fileURLToPath(
  new URL('../../../backend/api/openapi.yaml', import.meta.url),
);

describe('OpenAPI рецептов', () => {
  it('описывает шесть защищённых операций с no-store во всех ответах', async () => {
    const document = await SwaggerParser.validate(contractPath);
    if (!('components' in document)) throw new Error('Ожидался OpenAPI 3');
    const operations: string[] = [];
    for (const [path, entry] of Object.entries(document.paths ?? {})) {
      if (path !== '/recipe-categories' && !path.startsWith('/recipes'))
        continue;
      for (const method of ['get', 'post', 'put', 'patch', 'delete'] as const) {
        const operation = entry?.[method];
        if (!operation) continue;
        operations.push(`${method.toUpperCase()} ${path}`);
        expect(operation.security).toEqual([{ bearerAuth: [] }]);
        expect(operation.responses?.['401']).toBeDefined();
        expect(operation.responses?.['403']).toBeDefined();
        for (const response of Object.values(operation.responses ?? {})) {
          if (!('headers' in response)) throw new Error('Нет headers ответа');
          expect(response.headers?.['Cache-Control']).toMatchObject({
            schema: { type: 'string', enum: ['no-store'] },
          });
        }
      }
    }
    expect(operations.sort()).toEqual(
      [
        'GET /recipe-categories',
        'GET /recipes',
        'POST /recipes',
        'GET /recipes/{recipeId}',
        'PUT /recipes/{recipeId}',
        'DELETE /recipes/{recipeId}',
      ].sort(),
    );
    expect(document.paths?.['/recipes']?.post?.parameters).toContainEqual(
      expect.objectContaining({
        name: 'Idempotency-Key',
        in: 'header',
        required: true,
      }),
    );
    expect(
      document.paths?.['/recipes/{recipeId}']?.put?.parameters,
    ).toBeUndefined();
    for (const method of ['post', 'put'] as const) {
      const path = method === 'post' ? '/recipes' : '/recipes/{recipeId}';
      expect(
        document.paths?.[path]?.[method]?.responses?.['413'],
      ).toBeDefined();
      expect(document.paths?.[path]?.[method]?.requestBody).toMatchObject({
        required: true,
        'x-max-body-bytes': 1048576,
      });
    }
    expect(
      document.paths?.['/recipes']?.post?.responses?.['201'],
    ).toBeDefined();
    expect(
      document.paths?.['/recipes']?.post?.responses?.['409'],
    ).toBeDefined();
    expect(
      document.paths?.['/recipes/{recipeId}']?.delete?.responses?.['204'],
    ).not.toHaveProperty('content');
  });

  it('задаёт полное сохранение, строгие вложенные объекты и согласованные лимиты', async () => {
    const document = await SwaggerParser.dereference(contractPath);
    if (!('components' in document)) throw new Error('Ожидался OpenAPI 3');
    const schemas = document.components?.schemas;
    expect(schemas?.SaveRecipe).toMatchObject({
      type: 'object',
      additionalProperties: false,
      required: [
        'title',
        'cookingTimeMinutes',
        'categoryCode',
        'ingredients',
        'steps',
      ],
      properties: {
        title: { type: 'string', minLength: 1, maxLength: 200 },
        cookingTimeMinutes: {
          type: 'integer',
          minimum: 5,
          maximum: 1440,
          multipleOf: 5,
        },
        ingredients: { type: 'array', minItems: 1, maxItems: 200 },
        steps: { type: 'array', minItems: 1, maxItems: 100 },
      },
    });
    expect(schemas?.RecipeIngredientInput).toMatchObject({
      additionalProperties: false,
      required: ['name'],
      properties: {
        quantity: { type: 'string', nullable: true },
        unit: { type: 'string', maxLength: 32 },
      },
    });
    expect(schemas?.RecipeStepInput).toMatchObject({
      additionalProperties: false,
      required: ['text'],
      properties: { text: { type: 'string', minLength: 1, maxLength: 20000 } },
    });
    expect(schemas?.RecipeStepText).toHaveProperty(
      'description',
      expect.stringContaining('50000'),
    );
    for (const name of [
      'SaveRecipe',
      'RecipeIngredientInput',
      'RecipeStepInput',
    ]) {
      const schema = schemas?.[name];
      if (!schema || !('properties' in schema)) throw new Error(`Нет ${name}`);
      for (const field of [
        'id',
        'ownerId',
        'recipeId',
        'position',
        'createdAt',
        'updatedAt',
        'cover',
      ]) {
        expect(schema.properties).not.toHaveProperty(field);
      }
    }
    expect(schemas?.RecipeCategoryCode).toHaveProperty('enum', [
      'soups',
      'salads',
      'starters',
      'main-dishes',
      'side-dishes',
      'baking',
      'desserts',
      'sauces',
      'drinks',
      'other',
    ]);
  });

  it('разделяет краткий и полный DTO со строковыми ID и количествами', async () => {
    const document = await SwaggerParser.dereference(contractPath);
    if (!('components' in document)) throw new Error('Ожидался OpenAPI 3');
    const schemas = document.components?.schemas;
    const summary = schemas?.RecipeSummary;
    const full = schemas?.Recipe;
    if (
      !summary ||
      !full ||
      !('properties' in summary) ||
      !('properties' in full)
    ) {
      throw new Error('Нет DTO рецепта');
    }
    expect(summary.additionalProperties).toBe(false);
    expect(full.additionalProperties).toBe(false);
    expect(Object.keys(summary.properties ?? {}).sort()).toEqual(
      [
        'id',
        'title',
        'cookingTimeMinutes',
        'categoryCode',
        'createdAt',
        'updatedAt',
      ].sort(),
    );
    expect(full.required).toEqual([
      ...(summary.required ?? []),
      'ingredients',
      'steps',
    ]);
    for (const [name, property] of Object.entries(summary.properties ?? {})) {
      expect(full.properties?.[name]).toEqual(property);
    }
    expect(full.properties?.id).toMatchObject({
      type: 'string',
      maxLength: 19,
    });
    expect(schemas?.RecipeIngredient).toMatchObject({
      required: ['position', 'name', 'quantity', 'unit'],
      properties: {
        position: { type: 'integer', minimum: 1 },
        quantity: { type: 'string', nullable: true },
      },
    });
    expect(schemas?.RecipeStep).toMatchObject({
      required: ['position', 'text'],
    });
    expect(schemas?.RecipeCategory).toMatchObject({
      additionalProperties: false,
      required: ['code', 'name'],
    });
  });
});
