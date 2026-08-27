//! Type rules over the optional front-matter keys.

use crate::lint::rule::Rule;
use crate::lint::{FileContext, FindingSink, RULE_ALLOWED_TOOLS_TYPE, RULE_METADATA_TYPE};
use crate::parse::Value;

pub struct AllowedToolsType;

impl Rule for AllowedToolsType {
    fn id(&self) -> &'static str {
        RULE_ALLOWED_TOOLS_TYPE
    }

    fn check(&self, ctx: &FileContext<'_>, sink: &mut FindingSink<'_>) {
        let Some(fm) = ctx.frontmatter() else { return };
        if !fm.has("allowed-tools") {
            return;
        }
        if fm.string_slice("allowed-tools").is_none() {
            sink.error(
                fm.line("allowed-tools"),
                "allowed-tools must be a list of tool names or a comma-separated string"
                    .to_string(),
            );
        }
    }
}

pub struct MetadataType;

impl Rule for MetadataType {
    fn id(&self) -> &'static str {
        RULE_METADATA_TYPE
    }

    fn check(&self, ctx: &FileContext<'_>, sink: &mut FindingSink<'_>) {
        let Some(fm) = ctx.frontmatter() else { return };
        match fm.node("metadata") {
            None | Some(Value::Mapping) => {}
            Some(_) => sink.error(
                fm.line("metadata"),
                "metadata must be a mapping of keys to values".to_string(),
            ),
        }
    }
}
