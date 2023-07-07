pipeline {
    agent { label 'insights' }
    options {
        timestamps()
    }
    
    stages {
        stage('Run Unit Tests') {
            steps {
                sh '''
                    . ./ci/helpers.sh
                    . ./ci/unit_tests.sh
                '''
            }
        }
    }

    post { 
        always {
            archiveArtifacts artifacts: 'artifacts/**/*', fingerprint: true
            junit skipPublishingChecks: true, testResults: 'artifacts/junit-eno.xml'
        }
    }
}
