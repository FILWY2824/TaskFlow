use std::path::PathBuf;

fn main() {
    let manifest_dir = PathBuf::from(std::env::var("CARGO_MANIFEST_DIR").unwrap());
    let project_root = manifest_dir
        .parent()
        .and_then(|p| p.parent())
        .expect("src-tauri should be under windows/src-tauri");
    let env_path = project_root.join(".env");

    println!("cargo:rerun-if-changed={}", env_path.display());

    if let Ok(contents) = std::fs::read_to_string(&env_path) {
        let env = parse_env(&contents);
        let default_server = env
            .iter()
            .find(|(key, _)| key == "TASKFLOW_DEFAULT_SERVER_URL")
            .or_else(|| env.iter().find(|(key, _)| key == "VITE_TASKFLOW_DEFAULT_SERVER"))
            .or_else(|| env.iter().find(|(key, _)| key == "PUBLIC_BASE_URL"))
            .map(|(_, value)| value.trim().trim_end_matches('/').to_string())
            .unwrap_or_default();

        if !default_server.is_empty() {
            println!("cargo:rustc-env=TASKFLOW_DEFAULT_SERVER_URL={default_server}");
        }
    }

    tauri_build::build();
}

fn parse_env(contents: &str) -> Vec<(String, String)> {
    let mut parsed: Vec<(String, String)> = Vec::new();

    for raw in contents.lines() {
        let line = raw.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let Some((key, value)) = line.split_once('=') else {
            continue;
        };
        let key = key.trim().to_string();
        let mut value = value.trim().trim_matches('"').trim_matches('\'').to_string();

        for (existing_key, existing_value) in &parsed {
            value = value.replace(&format!("${{{existing_key}}}"), existing_value);
            value = value.replace(&format!("${existing_key}"), existing_value);
        }

        parsed.push((key, value));
    }

    parsed
}

#[cfg(test)]
mod tests {
    use super::parse_env;

    #[test]
    fn parses_default_server_values_derived_from_public_base_url() {
        let env = parse_env(
            r#"
PUBLIC_BASE_URL=https://root.example.test/
TASKFLOW_DEFAULT_SERVER_URL=${PUBLIC_BASE_URL}
VITE_TASKFLOW_DEFAULT_SERVER=${TASKFLOW_DEFAULT_SERVER_URL}
"#,
        );

        assert_eq!(
            env.iter()
                .find(|(key, _)| key == "TASKFLOW_DEFAULT_SERVER_URL")
                .map(|(_, value)| value.as_str()),
            Some("https://root.example.test/")
        );
        assert_eq!(
            env.iter()
                .find(|(key, _)| key == "VITE_TASKFLOW_DEFAULT_SERVER")
                .map(|(_, value)| value.as_str()),
            Some("https://root.example.test/")
        );
    }
}
