import { ApiError } from './api';
import { has, i18n, t } from './i18n.svelte';

// The API reports validation problems in English; these are the German texts.
const validationDE: Record<string, string> = {
  'URL is required': 'Die URL fehlt.',
  'URL must start with http:// or https:// and must not contain credentials':
    'Die URL muss mit http:// oder https:// beginnen und darf keine Zugangsdaten enthalten.',
  'topic is required and may not contain / ? #': 'Das Topic fehlt oder enthält eines der Zeichen / ? #.',
  'token is required': 'Das Token fehlt.',
  'app token and user key are required': 'App-Token und User-Key werden benötigt.',
  'bot token and chat ID are required': 'Bot-Token und Chat-ID werden benötigt.',
  'invalid header name': 'Der Header-Name ist ungültig.',
  'SMTP server, sender and recipient are required': 'SMTP-Server, Absender und Empfänger werden benötigt.',
  'invalid port': 'Der Port ist ungültig.',
  'security must be starttls, tls or none': 'Bitte STARTTLS, TLS oder „keine“ als Verschlüsselung wählen.',
  'invalid e-mail address': 'Eine E-Mail-Adresse ist ungültig.',
  'unknown channel kind': 'Unbekannter Dienst.',
  'max. 64 characters': 'höchstens 64 Zeichen',
  'must be de or en': 'nur Deutsch oder Englisch',
  'unknown time zone': 'unbekannte Zeitzone',
  'max. 8': 'höchstens 8',
  'invalid name': 'ungültiger Name',
  '30-3600 seconds': '30 bis 3600 Sekunden',
  '60-86400 seconds': '60 bis 86400 Sekunden',
  'must be at least twice the scan interval': 'muss mindestens doppelt so lang sein wie das Scan-Intervall',
  '0-365': '0 bis 365',
  '7-3650': '7 bis 3650',
  '0-90': '0 bis 90',
  '1-5 entries': '1 bis 5 Einträge',
  'use host:port': 'bitte im Format host:port',
  'invalid host': 'ungültige Adresse',
  '1-65535': '1 bis 65535',
  'invalid credentials': 'ungültige Zugangsdaten',
  'topics may only contain a-z, 0-9, _ - /': 'Topics dürfen nur a–z, 0–9, _ - / enthalten',
};

const fieldDE: Record<string, string> = {
  networkName: 'Netzwerkname',
  language: 'Sprache',
  timezone: 'Zeitzone',
  interfaces: 'Schnittstellen',
  scanInterval: 'Scan-Intervall',
  offlineAfter: 'Offline nach',
  offlineAfterMobile: 'Offline nach (Mobilgeräte)',
  expireRandomDays: 'Private Adressen vergessen nach',
  retentionDays: 'Aufbewahrung',
  securityScanDays: 'Sicherheitscheck',
  internetTargets: 'Prüfziele',
  mqtt: 'MQTT',
  'mqtt.host': 'MQTT-Broker',
  'mqtt.port': 'MQTT-Port',
  'mqtt.password': 'das MQTT-Passwort',
  fritzbox: 'FRITZ!Box',
  'fritzbox.host': 'FRITZ!Box-Adresse',
  'fritzbox.password': 'das FRITZ!Box-Passwort',
  token: 'das Token',
  user: 'den User-Key',
  url: 'die Webhook-URL',
  headerValue: 'den Header-Wert',
  password: 'das Passwort',
};

function translateLine(line: string): string {
  if (i18n.lang !== 'de') return line;
  const tooLong = /^(\w+) is invalid or too long$/.exec(line);
  if (tooLong) return `„${fieldDE[tooLong[1]] ?? tooLong[1]}“ ist ungültig oder zu lang.`;
  if (line.startsWith('invalid configuration')) return 'Die Konfiguration ist ungültig.';
  const field = /^([\w.]+): (.+)$/.exec(line);
  if (field) return `${fieldDE[field[1]] ?? field[1]}: ${validationDE[field[2]] ?? field[2]}`;
  return validationDE[line] ?? line;
}

/** A user-facing message for any error thrown by the API client. */
export function errorText(e: unknown): string {
  if (!(e instanceof ApiError)) return t('err.network');
  if (e.code === 'secret_required') {
    const f = e.detail;
    return t('err.secret_required', { field: i18n.lang === 'de' ? (fieldDE[f] ?? f) : f });
  }
  if ((e.code === 'invalid_config' || e.code === 'invalid_settings') && e.detail) {
    return e.detail.split('\n').map(translateLine).join(' ');
  }
  const key = `err.${e.code}`;
  return has(key) ? t(key, { s: e.retryAfter, msg: e.detail }) : t('err.internal');
}
