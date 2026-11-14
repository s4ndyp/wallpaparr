/// <reference path="../pb_data/types.d.ts" />
/*
 * Server-side rotatie (fase 2): elke minuut kijkt de cron welke schermen aan de beurt
 * zijn en zet een nieuwe wallpaper op het scherm-record. De client pollt dat record en
 * zet het beeld — de server is de baas, de client alleen speler.
 *
 * Handmatige pins (bron = "handmatig") blijven staan tot je in de web-UI weer
 * "rotatie" kiest. Alle staat zit in de records, dus een herstart verliest niets.
 *
 * Let op: de logica staat in rotatie.js omdat PocketBase een hook in een eigen
 * JS-runtime draait — functies uit dit bestand zijn daar niet zichtbaar.
 */

cronAdd("wallpaper-rotatie", "* * * * *", () => {
  const rotatie = require(__hooks + "/rotatie.js");
  try {
    rotatie.wisselSchermen();
  } catch (e) {
    console.error("wallpaper-rotatie mislukt: " + e);
  }
});
