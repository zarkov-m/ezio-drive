/**
 * Settings Module - Environment configuration for Search Console + Indexing
 */

import { config } from 'dotenv';
import { join } from 'path';

config();

export interface Settings {
  /** Optional: GA4 property id (kept for metadata compatibility) */
  propertyId: string;
  /** Service account email */
  clientEmail: string;
  /** Service account private key */
  privateKey: string;
  /** Default date range for reports (e.g., "30d") */
  defaultDateRange: string;
  /** Directory path for storing results */
  resultsDir: string;
  /** Search Console site URL (e.g., https://example.com) */
  siteUrl: string;
}

export interface ValidationResult {
  valid: boolean;
  errors: string[];
}

export function getSettings(): Settings {
  return {
    propertyId: process.env.GA4_PROPERTY_ID || process.env.SEARCH_CONSOLE_SITE_URL || 'search-console',
    clientEmail: process.env.GA4_CLIENT_EMAIL || '',
    privateKey: (process.env.GA4_PRIVATE_KEY || '').replace(/\\n/g, '\n'),
    defaultDateRange: process.env.GA4_DEFAULT_DATE_RANGE || '30d',
    resultsDir: join(process.cwd(), 'results'),
    siteUrl: process.env.SEARCH_CONSOLE_SITE_URL || '',
  };
}

export function validateSettings(): ValidationResult {
  const settings = getSettings();
  const errors: string[] = [];

  if (!settings.clientEmail) {
    errors.push('GA4_CLIENT_EMAIL is required');
  }
  if (!settings.privateKey) {
    errors.push('GA4_PRIVATE_KEY is required');
  }
  if (!settings.siteUrl) {
    errors.push('SEARCH_CONSOLE_SITE_URL is required');
  }

  return { valid: errors.length === 0, errors };
}
