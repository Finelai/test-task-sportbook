import assert from "node:assert/strict";
import { test } from "node:test";
import { Linter } from "eslint";
import tseslint from "typescript-eslint";
import { fsdRule } from "./fsd-rule.js";

const config = [
  {
    files: ["**/*.{js,ts}"],
    languageOptions: { parser: tseslint.parser },
    plugins: { local: { rules: { fsd: fsdRule } } },
    rules: { "local/fsd": "error" },
  },
];

for (const [filename, source, expected] of [
  [
    "pages/home/page.ts",
    'type Match = import("@/entities/match/model").Match;',
    "publicApi",
  ],
  [
    "pages/home/page.ts",
    'import type { Match } from "@/entities/match";',
    undefined,
  ],
  ["entities/match/model.js", 'import "@/features/place-bet";', "upward"],
  ["features/top-up/form.js", 'import "../place-bet/index.js";', "sameLayer"],
  ["pages/home/page.js", 'import "@/entities/match/model.js";', "publicApi"],
  [
    "pages/home/page.js",
    'export { value } from "@/entities/match/model.js";',
    "publicApi",
  ],
  [
    "pages/home/page.js",
    'void import("@/entities/match/model.js");',
    "publicApi",
  ],
  ["pages/home/page.js", 'import "@/entities/match";', undefined],
  ["pages/home/page.js", 'import "../../entities/match/index.ts";', undefined],
  ["entities/match/ui/card.js", 'import "../model.js";', undefined],
  ["entities/match/ui/card.js", 'import "@/shared/lib/format";', undefined],
  ["app/main.js", 'import "vue";', undefined],
]) {
  test(`${filename}: ${source}`, () => {
    const messages = new Linter().verify(source, config, {
      filename: `src/${filename}`,
    });
    assert.deepEqual(
      messages.map(({ messageId }) => messageId),
      expected ? [expected] : [],
    );
  });
}
