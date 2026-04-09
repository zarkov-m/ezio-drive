/**
 * Search Console + Indexing API clients (singleton)
 */

import { searchconsole } from '@googleapis/searchconsole';
import { indexing } from '@googleapis/indexing';
import { google } from 'googleapis';
import { getSettings, validateSettings } from '../config/settings.js';

let searchConsoleClientInstance: ReturnType<typeof searchconsole> | null = null;
let indexingClientInstance: ReturnType<typeof indexing> | null = null;

function getGoogleAuth() {
  const settings = getSettings();
  return new google.auth.GoogleAuth({
    credentials: {
      client_email: settings.clientEmail,
      private_key: settings.privateKey,
    },
    scopes: [
      'https://www.googleapis.com/auth/webmasters.readonly',
      'https://www.googleapis.com/auth/indexing',
    ],
  });
}

export function getSearchConsoleClient(): ReturnType<typeof searchconsole> {
  if (searchConsoleClientInstance) return searchConsoleClientInstance;

  const validation = validateSettings();
  if (!validation.valid) {
    throw new Error(`Invalid credentials: ${validation.errors.join(', ')}`);
  }

  const auth = getGoogleAuth();
  searchConsoleClientInstance = searchconsole({ version: 'v1', auth });
  return searchConsoleClientInstance;
}

export function getIndexingClient(): ReturnType<typeof indexing> {
  if (indexingClientInstance) return indexingClientInstance;

  const validation = validateSettings();
  if (!validation.valid) {
    throw new Error(`Invalid credentials: ${validation.errors.join(', ')}`);
  }

  const auth = getGoogleAuth();
  indexingClientInstance = indexing({ version: 'v3', auth });
  return indexingClientInstance;
}

export function getSiteUrl(): string {
  return getSettings().siteUrl;
}

export function resetClient(): void {
  searchConsoleClientInstance = null;
  indexingClientInstance = null;
}
