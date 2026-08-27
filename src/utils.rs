//! Small, general-purpose helpers shared across modules.
//!
//! Everything here is a pure function of its arguments: no I/O, no global
//! state, nothing specific to linting. Anything that needs to know what a rule
//! or a report is belongs in its own module instead.

use std::path::{Component, Path, PathBuf};

/// Renders n with thousands separators, so "6,142 tokens" reads at a glance
/// in reports.
pub fn humanize(n: i64) -> String {
    let s = n.to_string();
    if n < 0 {
        return s;
    }
    let bytes = s.as_bytes();
    let len = bytes.len();
    let mut out = String::with_capacity(len + len / 3);
    for (i, b) in bytes.iter().enumerate() {
        if i > 0 && (len - i).is_multiple_of(3) {
            out.push(',');
        }
        out.push(*b as char);
    }
    out
}

/// Formats a count with the right noun, as "1 file" or "2 files".
pub fn plural(n: usize, one: &str, many: &str) -> String {
    if n == 1 {
        format!("{n} {one}")
    } else {
        format!("{n} {many}")
    }
}

/// Renders a path with forward slashes, so output is identical on Windows.
pub fn to_slash(path: &Path) -> String {
    path.to_string_lossy().replace('\\', "/")
}

/// Lexically normalizes a path: resolves `.` and `..` textually, without
/// touching the filesystem or following symlinks (mirroring Go's
/// `filepath.Clean`). `..` above the root is dropped rather than escaping it.
pub fn clean_path(path: &Path) -> PathBuf {
    let mut out: Vec<Component> = Vec::new();
    for comp in path.components() {
        match comp {
            Component::CurDir => {}
            Component::ParentDir => match out.last() {
                Some(Component::Normal(_)) => {
                    out.pop();
                }
                Some(Component::RootDir) => {}
                _ => out.push(comp),
            },
            other => out.push(other),
        }
    }
    let mut result = PathBuf::new();
    for c in out {
        result.push(c.as_os_str());
    }
    if result.as_os_str().is_empty() {
        PathBuf::from(".")
    } else {
        result
    }
}

/// Divides, rounding up. Zero over anything is zero.
pub fn ceil_div(a: usize, b: usize) -> usize {
    if a == 0 { 0 } else { a.div_ceil(b) }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn humanize_formats_with_separators() {
        let cases: &[(i64, &str)] = &[
            (0, "0"),
            (7, "7"),
            (999, "999"),
            (1000, "1,000"),
            (6142, "6,142"),
            (1234567, "1,234,567"),
            (-42, "-42"),
        ];
        for (n, want) in cases {
            assert_eq!(humanize(*n), *want, "humanize({n})");
        }
    }

    #[test]
    fn plural_picks_the_noun() {
        assert_eq!(plural(0, "file", "files"), "0 files");
        assert_eq!(plural(1, "file", "files"), "1 file");
        assert_eq!(plural(2, "file", "files"), "2 files");
    }

    #[test]
    fn clean_path_normalizes_lexically() {
        let cases: &[(&str, &str)] = &[
            ("a/./b", "a/b"),
            ("a/b/../c", "a/c"),
            ("./a", "a"),
            (".", "."),
            ("", "."),
            ("/a/../../b", "/b"),
            ("../a", "../a"),
        ];
        for (input, want) in cases {
            assert_eq!(clean_path(Path::new(input)), PathBuf::from(want), "{input}");
        }
    }

    #[test]
    fn ceil_div_rounds_up() {
        assert_eq!(ceil_div(0, 4), 0);
        assert_eq!(ceil_div(1, 4), 1);
        assert_eq!(ceil_div(4, 4), 1);
        assert_eq!(ceil_div(5, 4), 2);
    }
}
