#!/usr/bin/env node
'use strict';

const Decimal = require('decimal.js');
const zlib = require('zlib');
const crypto = require('crypto');
const model = require('./upstream/data/model.js');
const calculator = require('./upstream/calc.js');
const sourceData = require('./upstream/data/ClickerHeroes_v14307.json');

const HEADERS = {
  '7a990d405d2c6fb93aa8fbb0ec1a3b23': 'zlib',
  '7e8bb5a89f2842ac4af01b3b7e228592': 'deflate'
};
const MAX_SAVE_LENGTH = 4 * 1024 * 1024;
const MAX_DECOMPRESSED_LENGTH = 16 * 1024 * 1024;
const MAX_EXPONENT = 100000;

function decodeSave(save) {
  if (typeof save !== 'string' || save.length === 0 || save.length > MAX_SAVE_LENGTH) {
    throw new Error('save must be a non-empty string of at most 4 MiB');
  }
  save = save.replace(/^\uFEFF/, '').trim();
  const header = save.slice(0, 32);
  let json;
  if (HEADERS[header]) {
    if (!/^[A-Za-z0-9+/]*={0,2}$/.test(save.slice(32)) || save.slice(32).length % 4 === 1) throw new Error('invalid save base64');
    let bytes;
    try { bytes = Buffer.from(save.slice(32), 'base64'); } catch (_) { throw new Error('invalid save base64'); }
    if (!bytes.length) throw new Error('invalid save payload');
    try {
      const out = HEADERS[header] === 'zlib' ? zlib.inflateSync(bytes, {maxOutputLength: MAX_DECOMPRESSED_LENGTH}) : zlib.inflateRawSync(bytes, {maxOutputLength: MAX_DECOMPRESSED_LENGTH});
      json = out.toString('utf8');
    } catch (_) { throw new Error('invalid compressed save'); }
  } else {
    // Official legacy "sprinkle" export: every second character is noise.
    let encoded = save;
    const marker = 'Fe12NAfA3R6z4k0z';
    if (encoded.includes('ClickerHeroesAccountSO')) encoded = encoded.slice(53, -1);
    else {
      encoded = encoded.trim();
      const markerAt = encoded.indexOf(marker);
      if (markerAt >= 0) {
        const parts = encoded.split(marker);
        const clean = Array.from({length: parts[0].length / 2}, (_, i) => parts[0][i * 2]).join('');
        const expected = crypto.createHash('md5').update(clean + 'af0ik392jrmt0nsfdghy0').digest('hex');
        if (expected !== parts[1]) throw new Error('invalid legacy save checksum');
        encoded = clean;
      }
    }
    try { json = Buffer.from(encoded, 'base64').toString('utf8'); } catch (_) { throw new Error('invalid legacy save'); }
  }
  try {
    const data = JSON.parse(json);
    if (!data || typeof data !== 'object') throw new Error();
    return data;
  } catch (_) { throw new Error('save payload is not valid JSON'); }
}

function decimal(value, field) {
  let result;
  try { result = new Decimal(value); } catch (_) { throw new Error(`${field} must be a finite non-negative number`); }
  if (!result.isFinite() || result.isNegative() || Math.abs(result.e || 0) > MAX_EXPONENT) throw new Error(`${field} must be a finite non-negative number`);
  return result;
}

function reserveAmount(value, souls) {
  if (typeof value === 'string' && value.trim().endsWith('%')) {
    const percent = decimal(value.trim().slice(0, -1), 'reserve percentage');
    if (percent.greaterThan(100)) throw new Error('reserve percentage must be from 0 to 100');
    return souls.times(percent).dividedBy(100);
  }
  return decimal(value, 'reserve');
}

function compactQuantity(value) {
  const exact = value.toString();
  if (exact.length <= 30) return exact;
  return new Decimal(value.toPrecision(15, Decimal.ROUND_DOWN)).floor().toString();
}

function run(input) {
  if (!input || typeof input !== 'object') throw new Error('input must be an object');
  if (typeof input.beyond8k !== 'boolean') throw new Error('beyond8k must be boolean');
  if (typeof input.skillRate !== 'number' || !Number.isFinite(input.skillRate) || input.skillRate < 0 || input.skillRate > 1) throw new Error('skillRate must be a number from 0 to 1');
  const skillRate = decimal(input.skillRate, 'skillRate');
  const raw = decodeSave(input.save);
  for (const key of ['heroSouls', 'heroSoulsSacrificed', 'highestFinishedZonePersist', 'ancientSoulsTotal']) {
    if (raw[key] === undefined) throw new Error(`save is missing ${key}`);
  }
  if (!raw.ancients || !raw.ancients.ancients || !raw.outsiders || !raw.outsiders.outsiders) throw new Error('save is missing ancient data');

  Decimal.set({precision: 100});
  global.data = JSON.parse(JSON.stringify(sourceData));
  model.createObjects(global.data);
  const data = global.data;
  data.settings = {
    buildMode: 'active', wep8k: input.beyond8k,
    hybridRatio: 1, skillAncientsLevelRate: skillRate.toString(),
    revolcLevelRate: 0, precision: 10, ignoreMinimizedAncients: false
  };
  data.heroSoulsSacrificed = decimal(raw.heroSoulsSacrificed, 'heroSoulsSacrificed');
  data.heroSouls = decimal(raw.heroSouls, 'heroSouls');
  const reserve = reserveAmount(input.reserve, data.heroSouls);
  if (reserve.greaterThan(data.heroSouls)) throw new Error('reserve exceeds available hero souls');
  data.heroSoulsForLeveling = data.heroSouls.minus(reserve);
  data.ascensionZone = decimal(raw.highestFinishedZonePersist, 'highestFinishedZonePersist');
  data.ancientSoulsTotal = decimal(raw.ancientSoulsTotal, 'ancientSoulsTotal');
  const ascensions = decimal(raw.numWorldResets, 'numWorldResets');
  if (!ascensions.isInteger()) throw new Error('numWorldResets must be an integer');
  if (ascensions.greaterThan(Number.MAX_SAFE_INTEGER)) throw new Error('numWorldResets exceeds safe integer range');
  data.ascensionSouls = new Decimal(0);
  data.tp = data.ancientSoulsTotal.times(-0.0003).exp().times(-0.23).plus(0.25).times(100);
  data.tp = Decimal.max(data.tp, 1);
  if (!raw.transcendent) data.tp = new Decimal(0);

  const ancientById = raw.ancients.ancients;
  for (const [id, entry] of Object.entries(ancientById)) {
    if (!/^\d+$/.test(id) || !entry || entry.level === undefined) throw new Error('malformed ancient data');
    const level = decimal(entry.level, `ancient ${id} level`);
    if (!level.isInteger()) throw new Error(`ancient ${id} level must be an integer`);
    if (!Object.values(sourceData.ancients).some(ancient => String(ancient.id) === id) && level.greaterThan(0)) throw new Error(`unknown owned ancient ${id}`);
  }
  for (const key of Object.keys(data.ancients)) {
    const ancient = data.ancients[key];
    ancient.level = decimal(ancientById[String(ancient.id)] ? ancientById[String(ancient.id)].level : 0, `ancient ${key} level`).floor();
    ancient.minimized = false;
  }
  const outsiderById = raw.outsiders.outsiders;
  for (const [id, entry] of Object.entries(outsiderById)) {
    if (!/^\d+$/.test(id) || !entry || entry.level === undefined) throw new Error('malformed outsider data');
    const level = decimal(entry.level, `outsider ${id} level`);
    if (!level.isInteger()) throw new Error(`outsider ${id} level must be an integer`);
    if (!Object.values(sourceData.outsiders).some(outsider => String(outsider.id) === id) && level.greaterThan(0)) throw new Error(`unknown owned outsider ${id}`);
  }
  for (const key of Object.keys(data.outsiders)) {
    const outsider = data.outsiders[key];
    outsider.level = decimal(outsiderById[String(outsider.id)] ? outsiderById[String(outsider.id)].level : 0, `outsider ${key} level`);
  }
  if (!data.ancients.fragsworth || !data.ancients.fragsworth.level.greaterThan(0)) throw new Error('Fragsworth must be owned');
  data.outsiders["chor'gorloth"] = data.outsiders["chor'gorloth"] || {level: new Decimal(0)};

  const spent = calculator.calculate();
  if (spent.isNegative() || spent.greaterThan(data.heroSoulsForLeveling)) throw new Error('calculated spend exceeds available souls');
  const rows = [];
  const owned = [];
  for (const key of Object.keys(data.ancients)) {
    const ancient = data.ancients[key];
    if (key === 'soulbank') continue;
    if (ancient.level.greaterThan(0)) owned.push({id: ancient.id, name: ancient.name.split(',')[0], level: ancient.level.toString()});
    const target = ancient.extraInfo.optimalLevel;
    if (!target || !ancient.level.greaterThan(0)) continue;
    const quantity = target.minus(ancient.level);
    const cost = ancient.extraInfo.costToLevelToOptimal || new Decimal(0);
    if (quantity.greaterThan(0) && cost.greaterThan(0)) rows.push({
      id: ancient.id, name: ancient.name.split(',')[0], current: ancient.level.toString(),
      target: target.toString(), quantity: compactQuantity(quantity), cost: cost.toString()
    });
  }
  return {souls: data.heroSouls.toString(), reserve: reserve.toString(), spent: spent.toString(),
    remaining: data.heroSouls.minus(spent).toString(), ascensions: ascensions.toNumber(), rows, owned};
}

if (require.main === module) {
  let input = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', chunk => { input += chunk; if (input.length > MAX_SAVE_LENGTH + 1000) process.exit(1); });
  process.stdin.on('end', () => {
    try { process.stdout.write(JSON.stringify(run(JSON.parse(input)))); }
    catch (error) { process.stderr.write(`ancient plan: ${error.message}\n`); process.exitCode = 1; }
  });
}

module.exports = {decodeSave, run};
