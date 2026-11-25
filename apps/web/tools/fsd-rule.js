import path from "node:path";

const layers = ["shared", "entities", "features", "widgets", "pages", "app"];
const slicedLayers = new Set(["entities", "features", "widgets", "pages"]);

export const fsdRule = {
  meta: {
    schema: [],
    messages: {
      upward: "Import from a lower FSD layer.",
      sameLayer: "Import only within the same slice at this layer.",
      publicApi: "Import a slice through its root index.ts.",
    },
  },
  create(context) {
    function check(source) {
      if (typeof source?.value !== "string") return;

      const filename = context.physicalFilename;
      const src = path.resolve(context.cwd, "src");
      const specifier = source.value.split(/[?#]/, 1)[0];
      let target;
      if (specifier.startsWith("@/")) {
        target = path.resolve(src, specifier.slice(2));
      } else if (specifier.startsWith(".")) {
        target = path.resolve(path.dirname(filename), specifier);
      } else if (specifier.startsWith("/src/")) {
        target = path.resolve(context.cwd, specifier.slice(1));
      } else {
        return;
      }

      const [fromLayer, fromSlice] = path
        .relative(src, filename)
        .split(path.sep);
      const [toLayer, toSlice, ...entry] = path
        .relative(src, target)
        .split(path.sep);
      const fromRank = layers.indexOf(fromLayer);
      const toRank = layers.indexOf(toLayer);
      if (fromRank < 0 || toRank < 0) return;

      if (toRank > fromRank) {
        context.report({ node: source, messageId: "upward" });
      } else if (
        fromLayer === toLayer &&
        slicedLayers.has(toLayer) &&
        fromSlice !== toSlice
      ) {
        context.report({ node: source, messageId: "sameLayer" });
      } else if (
        fromLayer !== toLayer &&
        slicedLayers.has(toLayer) &&
        (entry.length > 1 ||
          (entry.length === 1 && !/^index(?:\.ts)?$/.test(entry[0])))
      ) {
        context.report({ node: source, messageId: "publicApi" });
      }
    }

    return {
      ImportDeclaration: (node) => check(node.source),
      ExportNamedDeclaration: (node) => check(node.source),
      ExportAllDeclaration: (node) => check(node.source),
      ImportExpression: (node) => check(node.source),
      TSImportType: (node) => check(node.source),
      TSExternalModuleReference: (node) => check(node.expression),
    };
  },
};
