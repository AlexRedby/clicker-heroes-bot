'use strict';
const assert = require('assert');
const fs = require('fs');
const path = require('path');
const zlib = require('zlib');
const {run} = require('./plan.cjs');
const modelData = require('./upstream/data/ClickerHeroes_v14307.json');

const byName = name => Object.values(modelData.ancients).find(x => x.name.toLowerCase().startsWith(name + ','));
const outsider = name => Object.values(modelData.outsiders).find(x => x.name.toLowerCase() === name);
function save({souls = '1e6', chor = '0', spent = {}, omitSpent = false} = {}) {
  const ancient = (name, level) => [String(byName(name).id), {
    level: String(level),
    ...(omitSpent ? {} : {spentHeroSouls: String(spent[name] || '0')})
  }];
  const raw = {
    numWorldResets: 7,
    heroSouls: souls, heroSoulsSacrificed: '1e5', highestFinishedZonePersist: '1000', ancientSoulsTotal: '100',
    primalSouls: '0', transcendent: true, epicHeroReceivedUpTo: '90', extraGildsAwarded: '0',
    heroCollection: {heroes: {'1': {level: '0'}}}, ancientEntrySizes: {},
    ancients: {ancients: Object.fromEntries([ancient('fragsworth', 10), ancient('argaiv', 5), ancient('siyalatas', 4)])},
    outsiders: {outsiders: {[String(outsider("chor'gorloth").id)]: {level: chor}}}
  };
  const json = JSON.stringify(raw);
  return '7a990d405d2c6fb93aa8fbb0ec1a3b23' + zlib.deflateSync(json).toString('base64');
}

const input = {save: `\n\uFEFF ${save({chor: '0'})} \n`, reserve: '100', skillRate: 1, beyond8k: false};
const plain = run(input);
assert.equal(plain.souls, '1000000');
assert.equal(plain.reserve, '100');
assert.equal(plain.ascensions, 7);
assert.equal(plain.invested, '0');
assert(plain.rows.some(row => row.name === 'Argaiv' && row.current === '5'));
assert(!plain.rows.some(row => row.name === 'Siyalatas')); // active build excludes idle-only goals
assert(plain.owned.some(row => row.name === 'Fragsworth' && row.level === '10'));
assert(plain.rows.every(row => Number(row.quantity) > 0 && Number(row.cost) > 0));

const discounted = run({...input, save: save({chor: '3'})});
assert(discounted.rows.some((row, index) => row.target !== plain.rows[index].target));
const invested = run({...input, save: save({spent: {fragsworth: '12', argaiv: '3.5', siyalatas: '0'}})});
assert.equal(invested.invested, '15.5');
const unknownInvested = run({...input, save: save({omitSpent: true})});
assert.equal(unknownInvested.invested, '');
const huge = run({save: save({souls: '1e50', chor: '1'}), reserve: '1e20', skillRate: 0.5, beyond8k: true});
assert(/e\+/.test(huge.souls));
assert.throws(() => run({...input, save: 'corrupt'}), /save/);
assert.throws(() => run({...input, reserve: '-1'}), /reserve/);
assert.throws(() => run({...input, skillRate: 2}), /skillRate/);
assert.throws(() => run({...input, beyond8k: 1}), /beyond8k/);
const percent = run({...input, reserve: '1%'});
assert.equal(percent.reserve, '10000');
assert(new (require('decimal.js'))(percent.remaining).greaterThanOrEqualTo('10000'));
assert.throws(() => run({...input, reserve: '101%'}), /percentage/);
assert.throws(() => run({...input, save: save({}), reserve: '1e1000000'}), /finite/);
assert.throws(() => run({...input, save: save({}), reserve: '2e6'}), /reserve/);
assert.throws(() => run({...input, save: '7a990d405d2c6fb93aa8fbb0ec1a3b23' + require('zlib').deflateSync(Buffer.alloc(17 * 1024 * 1024)).toString('base64')}), /compressed/);

const real = run({save: fs.readFileSync(path.join(__dirname, '../../testdata/ancient-save.txt'), 'utf8'), reserve: '1%', skillRate: 1, beyond8k: false});
const Decimal = require('decimal.js');
assert.equal(real.owned.length, 26);
assert.equal(real.rows.length, 22);
assert(new Decimal(real.remaining).greaterThanOrEqualTo(real.reserve));
assert(real.rows.every(row => {
  const current = new Decimal(row.current), target = new Decimal(row.target), quantity = new Decimal(row.quantity);
  return quantity.isInteger() && quantity.greaterThan(0) && quantity.lessThanOrEqualTo(target.minus(current)) && current.toPrecision(4) !== target.toPrecision(4);
}));
console.log('ancient plan regression: ok');
