/// <reference path="../pb_data/types.d.ts" />
migrate(
  (app) => {
    const wallpapers = app.findCollectionByNameOrId("wallpapers");

    // Playlists: de selectie + het wisselritme voor (groepen) schermen.
    const playlists = new Collection({
      type: "base",
      name: "playlists",
      listRule: "",
      viewRule: "",
      createRule: "",
      updateRule: "",
      deleteRule: "",
      fields: [
        { name: "naam", type: "text", required: true, max: 120, presentable: true },
        // Kennen we "wallpapers" (handmatige selectie) gevuld, dan gaat die boven tags/ratios
        {
          name: "wallpapers",
          type: "relation",
          collectionId: wallpapers.id,
          maxSelect: 999,
          cascadeDelete: false,
        },
        { name: "tags", type: "text", max: 300 }, // kommagescheiden, leeg = alle
        { name: "ratios", type: "text", max: 200 }, // bv. "16:9,21:9", leeg = alle
        { name: "interval", type: "number", onlyInt: true }, // minuten, 0 = niet automatisch
        { name: "random", type: "bool" },
        { name: "actief", type: "bool" },
        { name: "created", type: "autodate", onCreate: true, onUpdate: false },
        { name: "updated", type: "autodate", onCreate: true, onUpdate: true },
      ],
    });
    app.save(playlists);

    // Schermen: elke monitor die een client meldt, met wat er nu op staat.
    const screens = new Collection({
      type: "base",
      name: "screens",
      listRule: "",
      viewRule: "",
      createRule: "",
      updateRule: "",
      deleteRule: "",
      fields: [
        { name: "key", type: "text", required: true, max: 200 }, // device|monitor|n
        { name: "device", type: "text", max: 120, presentable: true },
        { name: "monitor", type: "text", max: 120 },
        { name: "index", type: "number", onlyInt: true },
        { name: "width", type: "number", onlyInt: true },
        { name: "height", type: "number", onlyInt: true },
        { name: "laatst_gezien", type: "date" },
        {
          name: "playlist",
          type: "relation",
          collectionId: playlists.id,
          maxSelect: 1,
          cascadeDelete: false,
        },
        {
          name: "huidige_wallpaper",
          type: "relation",
          collectionId: wallpapers.id,
          maxSelect: 1,
          cascadeDelete: false,
        },
        // bron: rotatie = de cron mag wisselen, handmatig = pin, blijft staan
        { name: "bron", type: "select", maxSelect: 1, values: ["rotatie", "handmatig"] },
        { name: "gewijzigd", type: "date" },
        { name: "created", type: "autodate", onCreate: true, onUpdate: false },
        { name: "updated", type: "autodate", onCreate: true, onUpdate: true },
      ],
      indexes: ["CREATE UNIQUE INDEX `idx_screens_key` ON `screens` (`key`)"],
    });
    app.save(screens);

    // Geschiedenis: wat er daadwerkelijk getoond is (de client meldt dit).
    const geschiedenis = new Collection({
      type: "base",
      name: "geschiedenis",
      listRule: "",
      viewRule: "",
      createRule: "",
      updateRule: "",
      deleteRule: "",
      fields: [
        { name: "screen", type: "relation", collectionId: screens.id, maxSelect: 1, cascadeDelete: false },
        {
          name: "wallpaper",
          type: "relation",
          collectionId: wallpapers.id,
          maxSelect: 1,
          cascadeDelete: false,
        },
        { name: "device", type: "text", max: 120 },
        { name: "bron", type: "select", maxSelect: 1, values: ["rotatie", "handmatig"] },
        { name: "toegepast", type: "date" },
        { name: "created", type: "autodate", onCreate: true, onUpdate: false },
        { name: "updated", type: "autodate", onCreate: true, onUpdate: true },
      ],
    });
    app.save(geschiedenis);
  },
  (app) => {
    for (const naam of ["geschiedenis", "screens", "playlists"]) {
      try {
        app.delete(app.findCollectionByNameOrId(naam));
      } catch (e) {
        // al weg
      }
    }
  }
);
