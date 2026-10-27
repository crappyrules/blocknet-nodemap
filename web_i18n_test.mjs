import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { messages, resolveLocale, translate } from './web/i18n.js';

test('browser language matching and English fallback', () => {
    assert.equal(resolveLocale(['es-MX', 'en-US']), 'es');
    assert.equal(resolveLocale(['zh-Hans-CN', 'en-US']), 'zh-CN');
    assert.equal(resolveLocale(['zh-TW', 'fr-FR']), 'fr');
    assert.equal(resolveLocale(['ja-JP', 'fr-FR']), 'fr');
    assert.equal(resolveLocale(['ja-JP']), 'en');
});

test('all interface keys are translated and placeholders match', () => {
    const english = Object.keys(messages.en).sort();
    for (const [locale, entries] of Object.entries(messages)) {
        assert.deepEqual(Object.keys(entries).sort(), english, `missing ${locale} translation`);
        for (const key of english) {
            const params = value => [...value.matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort();
            assert.deepEqual(params(entries[key]), params(messages.en[key]), `${locale}.${key} placeholders`);
        }
    }
    assert.equal(translate('es', 'updated', { time: '12:00' }), 'Actualizado a las 12:00');
});

test('interface translation references exist', () => {
    const html = readFileSync(new URL('./web/index.html', import.meta.url), 'utf8');
    const referenced = [
        ...html.matchAll(/data-i18n(?:-aria-label)?="([A-Za-z0-9]+)"/g),
        ...html.matchAll(/\bt\('([A-Za-z0-9]+)'/g),
    ].map(match => match[1]);
    for (const key of referenced) assert.ok(key in messages.en, `unknown translation key ${key}`);
    assert.equal(new Set(referenced).size > 20, true);
});
