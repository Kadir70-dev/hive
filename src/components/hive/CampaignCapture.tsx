'use client';
import { useEffect } from 'react';
import { captureCampaign } from '@/lib/campaign-attribution';
/** Reads only allowlisted UTM parameters; no tracking network request. */
export default function CampaignCapture() {
  useEffect(() => { try { captureCampaign(window.location.search, window.sessionStorage); } catch { /* storage disabled */ } }, []);
  return null;
}
