// @vitest-environment node
import SwaggerParser from '@apidevtools/swagger-parser';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const contractPath = fileURLToPath(new URL('../../../backend/api/shopping-lists.openapi.yaml', import.meta.url));

describe('Планируемый OpenAPI списков', () => {
  it('имеет валидную структуру и разрешимые ссылки', async () => {
    const document = await SwaggerParser.validate(contractPath);
    expect(document.info.version).toBe('0.2.0-planned');
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
    for (const value of ['0', '0.000', '-1', 'NaN', 'Infinity', '1e3', '1,5', '01', '.5', '1.0001', '1000000000', ' 1 ']) {
      expect(pattern.test(value), value).toBe(false);
    }
  });

});
