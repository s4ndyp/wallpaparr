/// <reference path="../pb_data/types.d.ts" />
migrate(
  (app) => {
    const collection = new Collection({
      type: "base",
      name: "wallpapers",
      // Alles publiek: geen login nodig.
      listRule: "",
      viewRule: "",
      createRule: "",
      updateRule: "",
      deleteRule: "",
      fields: [
        {
          name: "title",
          type: "text",
          required: true,
          min: 1,
          max: 200,
          presentable: true,
        },
        {
          name: "file",
          type: "file",
          required: true,
          maxSelect: 1,
          maxSize: 26214400, // 25 MB
          mimeTypes: [
            "image/jpeg",
            "image/png",
            "image/webp",
            "image/avif",
            "image/gif",
            "image/bmp",
          ],
          thumbs: ["400x0", "1600x0"],
        },
        {
          // Kommagescheiden tags, filterbaar met filter=(tags~'natuur')
          name: "tags",
          type: "text",
          max: 300,
        },
        { name: "width", type: "number", onlyInt: true },
        { name: "height", type: "number", onlyInt: true },
        { name: "bytes", type: "number", onlyInt: true },
        { name: "source", type: "text", max: 300 },
        {
          name: "created",
          type: "autodate",
          onCreate: true,
          onUpdate: false,
        },
        {
          name: "updated",
          type: "autodate",
          onCreate: true,
          onUpdate: true,
        },
      ],
      indexes: [
        "CREATE INDEX idx_wallpapers_tags ON wallpapers (tags)",
        "CREATE INDEX idx_wallpapers_created ON wallpapers (created)",
      ],
    });

    app.save(collection);
  },
  (app) => {
    const collection = app.findCollectionByNameOrId("wallpapers");
    app.delete(collection);
  }
);
